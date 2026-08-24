package handlers

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/models"
	"vauln-address/internal/repository"
	"vauln-address/internal/services"
	"vauln-address/internal/validators"
)

// ==================== Scanner ingest (solana_scan.py -> DB) ====================

// IngestScanFinding accepts findings from solana_scan.py (admin key required).
// Besides storing the finding it adds the involved wallets to the database:
// for DRAINER the victim as "drained" and the hacker as "hacker", for
// SUSPICIOUS the counterparty as "suspicious", so they show up in checks
// and reports. A hacker address that is one of the on-chain programs the
// transaction invoked is not registered: the funds only passed through the
// program, the real operator wallet sits behind it.
func (h *Handler) IngestScanFinding(c *gin.Context) {
	var req models.ScanFindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	req.Signature = strings.TrimSpace(req.Signature)
	if req.Verdict != models.ScanVerdictDrainer && req.Verdict != models.ScanVerdictSuspicious {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "verdict must be DRAINER or SUSPICIOUS",
			Code:  "INVALID_VERDICT",
		})
		return
	}
	if req.Chain == "" {
		req.Chain = string(models.ChainSolana)
	}
	// Besides the canonical chains the scanner may report a specific EVM
	// network (bnb, base, linea, …): the finding keeps it for display, but
	// address validation and wallet registration use the canonical chain.
	if !models.IsScanChain(req.Chain) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "unsupported chain",
			Code:  "INVALID_CHAIN",
		})
		return
	}
	req.Chain = strings.ToLower(req.Chain)
	walletChain := models.CanonicalChain(req.Chain)

	// A finding where victim and hacker coincide is malformed: the scanner
	// could not resolve the real operator. Drop the hacker side instead of
	// registering the victim as its own hacker.
	if req.HackerAddress == req.VictimAddress {
		req.HackerAddress = ""
	}

	// Reject addresses that fail strict encoding checks (base58-32B for
	// Solana): the scanner occasionally emits placeholder strings that look
	// like addresses but decode to the wrong byte length. The finding itself
	// is still stored, but the bogus counterparty is not registered as a
	// wallet and never pollutes reports.
	if req.VictimAddress != "" {
		if ok, _ := validators.ValidateAddress(walletChain, req.VictimAddress); !ok {
			req.VictimAddress = ""
		}
	}
	if req.HackerAddress != "" {
		if ok, _ := validators.ValidateAddress(walletChain, req.HackerAddress); !ok {
			req.HackerAddress = ""
		}
	}
	filtered := req.ExposedAddresses[:0]
	for _, addr := range req.ExposedAddresses {
		if ok, _ := validators.ValidateAddress(walletChain, addr); ok {
			filtered = append(filtered, addr)
		}
	}
	req.ExposedAddresses = filtered

	// Sweep recipients go through the same strict validation: bogus
	// addresses are dropped, the victim is never its own recipient.
	sweeps := req.Sweeps[:0]
	seenSweeps := map[string]bool{}
	for _, sw := range req.Sweeps {
		if sw.Address == "" || sw.Address == req.VictimAddress ||
			sw.AmountSOL <= 0 || seenSweeps[sw.Address] {
			continue
		}
		if ok, _ := validators.ValidateAddress(walletChain, sw.Address); !ok {
			continue
		}
		seenSweeps[sw.Address] = true
		sweeps = append(sweeps, sw)
	}
	req.Sweeps = sweeps

	id, inserted, err := h.repo.InsertScanFinding(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "database error",
			Code:  "DB_ERROR",
		})
		return
	}

	// A takeover-only finding (account assigned to a new owner, no funds
	// swept in the same transaction) names the owning PROGRAM as the
	// hacker: the assign instruction never invokes the owner program, so
	// it may be missing from the programs list. A program id is never a
	// hacker wallet — the real recipient surfaces via fund-flow tracing.
	hackerIsProgram := isProgramAddress(req.HackerAddress, req.Programs) || isTakeoverOnlyFinding(req.Indicators)

	victimAdded, hackerAdded := false, false
	sweepsAdded := 0
	if inserted {
		switch req.Verdict {
		case models.ScanVerdictDrainer:
			victimAdded = h.registerScanWallet(c, req.VictimAddress, walletChain,
				models.StatusDrained, "drainer victim: funds swept by drainer transaction")
			if !hackerIsProgram {
				hackerAdded = h.registerScanWallet(c, req.HackerAddress, walletChain,
					models.StatusHacker, "drainer operator: received stolen funds or hijacked accounts")
			}
		case models.ScanVerdictSuspicious:
			if !hackerIsProgram {
				hackerAdded = h.registerScanWallet(c, req.HackerAddress, walletChain,
					models.StatusSuspicious, "suspicious drainer-like transaction, not yet confirmed as malicious")
			}
		}
		// The remaining recipients of a split drain each got a share of the
		// swept funds. Like one-off downstream recipients (F1) they are
		// flagged as suspicious; only the primary recipient is the operator.
		sweepsAdded = h.registerSweepRecipients(c, req, walletChain)
	}

	// Wallets that sent funds to the operator are flagged as associated with
	// a hacker, but their status is never escalated automatically: unknown
	// ones are registered as "unknown", existing statuses stay untouched.
	// A takeover program is never an "operator" — its takeover victims were
	// hijacked, they did not pay anyone, so program findings never create
	// associations at all.
	associated := 0
	// Live-block inflow findings (L1) name a threat-listed recipient as the
	// hacker: the senders funding it get the same association flag as the
	// senders funding a drainer operator.
	liveInflow := hasIndicator(req.Indicators, "L1_WATCHED_INFLOW")
	if (req.Verdict == models.ScanVerdictDrainer || liveInflow) && req.HackerAddress != "" && !hackerIsProgram {
		reason := "transferred funds to known drainer operator " + req.HackerAddress
		l1 := liveInflow && req.Verdict != models.ScanVerdictDrainer
		if l1 {
			reason = "transferred funds to known threat address " + req.HackerAddress
		}
		// A drain victim never funded its drainer — it is never flagged.
		// An L1 finding is the opposite: its victim field IS the sender who
		// funded the threat-listed address, so it must be flaggable.
		seen := map[string]bool{req.HackerAddress: true}
		if !l1 {
			seen[req.VictimAddress] = true
		}
		for _, addr := range req.ExposedAddresses {
			if addr == "" || seen[addr] {
				continue
			}
			seen[addr] = true
			if err := h.repo.MarkAssociatedHacker(c.Request.Context(), addr, walletChain, reason); err != nil {
				log.Printf("scanner: failed to mark association %s: %v", addr, err)
				continue
			}
			associated++
		}
	}

	// A solana drainer-pattern finding that hit no known drainer program
	// (no P5) came through an unknown program: forward it to the analysts
	// for review so the watchlist can be extended. Sent only once per
	// finding (on first insert), best-effort and non-blocking. Live-block
	// findings never carry programs, so they are not review candidates.
	needsReview := inserted && len(req.Programs) > 0 &&
		!hasIndicator(req.Indicators, "P5_KNOWN_DRAINER_PROGRAM")
	if needsReview {
		go h.forwardFindingForReview(req)
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           id,
		"inserted":     inserted,
		"victim_added": victimAdded,
		"hacker_added": hackerAdded,
		"sweeps_added": sweepsAdded,
		"associated":   associated,
		"needs_review": needsReview,
	})
}

// hasIndicator reports whether the finding carries the given indicator code.
func hasIndicator(indicators []string, code string) bool {
	for _, ind := range indicators {
		if ind == code {
			return true
		}
	}
	return false
}

// isTakeoverOnlyFinding reports whether the finding is a pure account
// takeover (P1) with no funds swept in the same transaction (no P2/P6).
// In such findings the scanner names the owning on-chain program as the
// hacker — a program id, not a wallet.
func isTakeoverOnlyFinding(indicators []string) bool {
	return hasIndicator(indicators, "P1_ACCOUNT_TAKEOVER") &&
		!hasIndicator(indicators, "P2_FULL_BALANCE_SWEEP") &&
		!hasIndicator(indicators, "P6_TOKEN_SWEEP")
}

// forwardFindingForReview sends a scanner finding to the team chat when the
// drainer pattern fired on a program that is not in the watchlist yet.
// Failures are logged, never fatal to the ingest request.
func (h *Handler) forwardFindingForReview(req models.ScanFindingRequest) {
	symbol := chainCurrency[req.Chain]
	if symbol == "" {
		symbol = strings.ToUpper(req.Chain)
	}
	text := fmt.Sprintf(
		"🔍 <b>Scanner review: drainer pattern on an unknown program</b>\n"+
			"Verdict: <b>%s</b>\n"+
			"Chain: <b>%s</b>\n"+
			"TX: <code>%s</code>\n"+
			"Indicators: %s\n"+
			"Victim: <code>%s</code>\n"+
			"Hacker: <code>%s</code>\n"+
			"Programs: <code>%s</code>\n"+
			"Amount: %.4f %s\n"+
			"The program is not in the drainer watchlist — review the transaction and add it if confirmed.",
		html.EscapeString(req.Verdict),
		html.EscapeString(strings.ToUpper(req.Chain)),
		html.EscapeString(req.Signature),
		html.EscapeString(strings.Join(req.Indicators, ", ")),
		html.EscapeString(orDash(req.VictimAddress)),
		html.EscapeString(orDash(req.HackerAddress)),
		html.EscapeString(orDash(strings.Join(req.Programs, ", "))),
		req.AmountSOL,
		symbol,
	)

	if err := h.telegram.Send(context.Background(), text); err != nil {
		if err == services.ErrTelegramNotConfigured {
			log.Printf("scanner review: telegram bot not configured, finding %s not forwarded", req.Signature)
		} else {
			log.Printf("scanner review: telegram send failed for %s: %v", req.Signature, err)
		}
	}
}

// registerScanWallet adds a wallet discovered by the scanner to the main
// registry, unless it is already there. Returns true when a row was created.
func (h *Handler) registerScanWallet(c *gin.Context, address, chain string, status models.WalletStatus, reason string) bool {
	if address == "" {
		return false
	}
	_, exists, err := h.repo.GetWalletByAddressAndChain(c.Request.Context(), address, chain)
	if err != nil || exists {
		return false
	}
	if _, err := h.repo.CreateWallet(c.Request.Context(), address, chain, status, reason, "solana_scan"); err != nil {
		log.Printf("scanner: failed to register wallet %s (%s): %v", address, status, err)
		return false
	}
	return true
}

// registerSweepRecipients registers the secondary recipients of a split
// drain (every sweep destination except the primary hacker): each of them
// received a share of the stolen funds in the same transaction, so they are
// flagged as suspicious without waiting for fund-flow tracing to catch up.
func (h *Handler) registerSweepRecipients(c *gin.Context, req models.ScanFindingRequest, chain string) int {
	added := 0
	seen := map[string]bool{req.VictimAddress: true, req.HackerAddress: true}
	for _, sw := range req.Sweeps {
		if sw.Address == "" || seen[sw.Address] ||
			isProgramAddress(sw.Address, req.Programs) {
			continue
		}
		seen[sw.Address] = true
		if h.registerScanWallet(c, sw.Address, chain, models.StatusSuspicious,
			"split drain recipient: received a share of the stolen funds") {
			added++
		}
	}
	return added
}

// ==================== Live monitoring ====================

// monitorFeedLimit caps the monitoring feed: only the latest 10 findings are
// ever served, older ones stay in the database but are not retrievable.
const monitorFeedLimit = 10

// GetMonitorFindings returns scanner findings for the live monitoring page.
// The feed is intentionally limited to the latest monitorFeedLimit rows —
// older history is not served. With after_id > 0 the endpoint returns only
// newer rows ascending, which the frontend uses for incremental live polling.
func (h *Handler) GetMonitorFindings(c *gin.Context) {
	var afterID int64
	if a := c.Query("after_id"); a != "" {
		if parsed, err := strconv.ParseInt(a, 10, 64); err == nil && parsed > 0 {
			afterID = parsed
		}
	}

	findings, err := h.repo.GetScanFindings(c.Request.Context(), afterID, 0, monitorFeedLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "database error",
			Code:  "DB_ERROR",
		})
		return
	}
	if findings == nil {
		findings = []models.ScanFinding{}
	}
	c.JSON(http.StatusOK, gin.H{
		"findings": findings,
		"count":    len(findings),
	})
}

// monitorStatsTTL bounds how often the full-table aggregate behind
// /api/monitor/stats is recomputed.
const monitorStatsTTL = 30 * time.Second

// GetMonitorStats returns aggregate counters for the live monitoring page.
// The aggregate scans the whole scan_findings table, so responses are
// served from a short-lived in-memory cache; the mutex doubles as
// single-flight, concurrent polls never trigger parallel recomputes.
func (h *Handler) GetMonitorStats(c *gin.Context) {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	if h.statsCache != nil && time.Since(h.statsCacheAt) < monitorStatsTTL {
		c.JSON(http.StatusOK, h.statsCache)
		return
	}
	stats, err := h.repo.GetScanStats(c.Request.Context())
	if err != nil {
		// a refresh failure keeps serving the last good snapshot
		if h.statsCache != nil {
			c.JSON(http.StatusOK, h.statsCache)
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "database error",
			Code:  "DB_ERROR",
		})
		return
	}
	h.statsCache = stats
	h.statsCacheAt = time.Now()
	c.JSON(http.StatusOK, stats)
}

// ==================== User drainer reports (captcha + Telegram) ====================

// GetCaptcha issues a one-time SVG captcha challenge for the report form.
func (h *Handler) GetCaptcha(c *gin.Context) {
	id, image := h.captcha.Generate()
	c.JSON(http.StatusOK, models.CaptchaResponse{CaptchaID: id, Image: image})
}

// SubmitDrainerReport accepts a user report about a drainer theft.
// The captcha is verified before anything is stored or forwarded.
func (h *Handler) SubmitDrainerReport(c *gin.Context) {
	var req models.DrainerReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	if !h.captcha.Verify(req.CaptchaID, req.CaptchaAnswer) {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "captcha verification failed",
			Code:    "CAPTCHA_INVALID",
			Details: "request a new captcha and enter the code from the image",
		})
		return
	}

	chain := strings.ToLower(strings.TrimSpace(req.Chain))
	if chain == "" {
		chain = string(models.ChainSolana)
	}
	if !models.IsValidChain(chain) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "unsupported chain",
			Code:  "INVALID_CHAIN",
		})
		return
	}

	site := strings.TrimSpace(req.SiteURL)
	if site != "" && !strings.HasPrefix(site, "http://") && !strings.HasPrefix(site, "https://") {
		site = "https://" + site
	}

	reporter := repository.AnonymousRequesterPrefix + c.ClientIP()
	if ua, exists := c.Get("userAddress"); exists {
		if addr, ok := ua.(string); ok && addr != "" {
			reporter = addr
		}
	}

	report := &models.DrainerReport{
		TxSignature: strings.TrimSpace(req.TxSignature),
		Chain:       chain,
		SiteURL:     site,
		Description: strings.TrimSpace(req.Description),
		Reporter:    reporter,
		Status:      "new",
	}

	id, err := h.repo.InsertDrainerReport(c.Request.Context(), report)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "database error",
			Code:  "DB_ERROR",
		})
		return
	}
	report.ID = id

	sent := h.forwardDrainerReportToTelegram(report)

	c.JSON(http.StatusOK, gin.H{
		"id":            id,
		"status":        report.Status,
		"telegram_sent": sent,
		"message":       "report submitted, our analysts will review it",
	})
}

// forwardDrainerReportToTelegram sends the report to the team chat and
// records the delivery flag. Failures are logged, not fatal to the request.
func (h *Handler) forwardDrainerReportToTelegram(report *models.DrainerReport) bool {
	text := fmt.Sprintf(
		"🚨 <b>New drainer report #%d</b>\n"+
			"Chain: <b>%s</b>\n"+
			"TX: <code>%s</code>\n"+
			"Site: %s\n"+
			"Description: %s\n"+
			"Reporter: <code>%s</code>",
		report.ID,
		strings.ToUpper(report.Chain),
		html.EscapeString(report.TxSignature),
		html.EscapeString(orDash(report.SiteURL)),
		html.EscapeString(orDash(report.Description)),
		html.EscapeString(report.Reporter),
	)

	ctx := context.Background()
	if err := h.telegram.Send(ctx, text); err != nil {
		if err == services.ErrTelegramNotConfigured {
			log.Printf("drainer report #%d: telegram bot not configured, stored only", report.ID)
		} else {
			log.Printf("drainer report #%d: telegram send failed: %v", report.ID, err)
		}
		return false
	}
	if err := h.repo.MarkDrainerReportTelegram(ctx, report.ID, true); err != nil {
		log.Printf("drainer report #%d: failed to mark telegram_sent: %v", report.ID, err)
	}
	return true
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// ==================== Status evidence chain ====================

// scanIndicatorMeta mirrors the pattern codes emitted by solana_scan.py
// (detect_patterns). It turns indicator tags into human-readable evidence.
var scanIndicatorMeta = map[string]struct {
	Title string
	Desc  string
}{
	"P1_ACCOUNT_TAKEOVER": {
		"Account takeover",
		"the wallet's ownership was assigned to an unknown program, handing control over to the attacker",
	},
	"P2_FULL_BALANCE_SWEEP": {
		"Full balance sweep",
		"at least 90% of the balance was transferred out in one transaction, leaving only rent dust",
	},
	"P3_UNKNOWN_PROGRAM": {
		"Unknown program call",
		"the transaction invoked a program outside the known legitimate set",
	},
	"P4_CONTROL_ACCOUNT": {
		"Control account created",
		"a zero-space program-owned account was created, a typical drainer control artifact",
	},
	"P5_KNOWN_DRAINER_PROGRAM": {
		"Known drainer program",
		"the transaction invoked a program from the drainer watchlist",
	},
	"F1_DOWNSTREAM_TRANSFER": {
		"Downstream transfer from drainer operator",
		"this wallet received funds from a drainer operator — possible accomplice or cash-out hop",
	},
	"F2_REPEAT_DOWNSTREAM": {
		"Repeated downstream transfers from drainer operators",
		"this wallet received funds from drainer operators two or more times — likely accomplice or another hacker wallet",
	},
	"L1_WATCHED_INFLOW": {
		"Transfer to a known threat address",
		"a live block scanner saw funds sent to this threat-listed wallet",
	},
}

// buildStatusEvidence composes the chain of evidence explaining the wallet
// status: registry listing, leaked key material and scanner indicators with
// the transactions they were detected in. The findings arrive prefetched
// from assembleReport (id DESC); only the newest few become evidence.
func (h *Handler) buildStatusEvidence(c *gin.Context, report *models.ReportResponse, findings []models.ScanFinding) []models.StatusEvidence {
	var evidence []models.StatusEvidence

	if report.Reason != "" {
		desc := report.Reason
		if report.Source != "" {
			desc += fmt.Sprintf(" (source: %s)", report.Source)
		}
		evidence = append(evidence, models.StatusEvidence{
			Code:        "registry",
			Title:       "Listed in the threat database",
			Description: desc,
			DetectedAt:  report.CreatedAt,
		})
	}

	if report.HasPK {
		evidence = append(evidence, models.StatusEvidence{
			Code:        "key_leak",
			Title:       "Private key exposed",
			Description: "the private key of this wallet is publicly available",
			DetectedAt:  report.CreatedAt,
		})
	}
	if report.HasSeed {
		evidence = append(evidence, models.StatusEvidence{
			Code:        "key_leak",
			Title:       "Seed phrase exposed",
			Description: "the mnemonic (seed) phrase of this wallet is publicly available",
			DetectedAt:  report.CreatedAt,
		})
	}
	for _, leak := range report.Leaks {
		evidence = append(evidence, models.StatusEvidence{
			Code:  "key_leak",
			Title: "Key material leaked",
			Description: fmt.Sprintf("a %s was found in %s",
				leakTypeLabel(leak.KeyType), orDash(leak.Source)),
			DetectedAt: leak.DiscoveredAt,
		})
	}

	const evidenceFindingsCap = 20
	if len(findings) > evidenceFindingsCap {
		findings = findings[:evidenceFindingsCap]
	}
	for _, f := range findings {
		role := "victim"
		counterparty := f.HackerAddress
		amount := f.AmountSOL
		if f.HackerAddress == report.Address {
			role = "hacker"
			counterparty = f.VictimAddress
		}
		// A secondary recipient of a split drain is neither the victim nor
		// the primary recipient: the counterparty is the drained wallet and
		// the amount is the share it actually received.
		if role == "victim" && f.VictimAddress != report.Address {
			for _, sw := range f.Sweeps {
				if sw.Address == report.Address {
					role = "recipient"
					counterparty = f.VictimAddress
					amount = sw.AmountSOL
					break
				}
			}
		}
		seen := map[string]bool{}
		for _, ind := range f.Indicators {
			if seen[ind] {
				continue
			}
			seen[ind] = true
			meta, ok := scanIndicatorMeta[ind]
			if !ok {
				meta = struct{ Title, Desc string }{ind, "detected by the drainer scanner"}
			}
			desc := fmt.Sprintf("%s — this wallet appears as the %s in transaction %s (verdict %s)",
				meta.Desc, role, f.Signature, f.Verdict)
			evidence = append(evidence, models.StatusEvidence{
				Code:         ind,
				Title:        meta.Title,
				Description:  desc,
				TxSignature:  f.Signature,
				Counterparty: counterparty,
				AmountSOL:    amount,
				DetectedAt:   f.CreatedAt,
			})
		}
	}
	return evidence
}

func leakTypeLabel(keyType string) string {
	if keyType == "seed" {
		return "mnemonic (seed) phrase"
	}
	return "private key"
}
