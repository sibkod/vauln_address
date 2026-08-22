package handlers

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"

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
	if !models.IsValidChain(req.Chain) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "unsupported chain",
			Code:  "INVALID_CHAIN",
		})
		return
	}

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
		if ok, _ := validators.ValidateAddress(req.Chain, req.VictimAddress); !ok {
			req.VictimAddress = ""
		}
	}
	if req.HackerAddress != "" {
		if ok, _ := validators.ValidateAddress(req.Chain, req.HackerAddress); !ok {
			req.HackerAddress = ""
		}
	}
	filtered := req.ExposedAddresses[:0]
	for _, addr := range req.ExposedAddresses {
		if ok, _ := validators.ValidateAddress(req.Chain, addr); ok {
			filtered = append(filtered, addr)
		}
	}
	req.ExposedAddresses = filtered

	id, inserted, err := h.repo.InsertScanFinding(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "database error",
			Code:  "DB_ERROR",
		})
		return
	}

	victimAdded, hackerAdded := false, false
	if inserted {
		switch req.Verdict {
		case models.ScanVerdictDrainer:
			victimAdded = h.registerScanWallet(c, req.VictimAddress, req.Chain,
				models.StatusDrained, "drainer victim: funds swept by drainer transaction")
			if !isProgramAddress(req.HackerAddress, req.Programs) {
				hackerAdded = h.registerScanWallet(c, req.HackerAddress, req.Chain,
					models.StatusHacker, "drainer operator: received stolen funds or hijacked accounts")
			}
		case models.ScanVerdictSuspicious:
			if !isProgramAddress(req.HackerAddress, req.Programs) {
				hackerAdded = h.registerScanWallet(c, req.HackerAddress, req.Chain,
					models.StatusSuspicious, "suspicious drainer-like transaction, not yet confirmed as malicious")
			}
		}
	}

	// Wallets that sent funds to the operator are flagged as associated with
	// a hacker, but their status is never escalated automatically: unknown
	// ones are registered as "unknown", existing statuses stay untouched.
	associated := 0
	if req.Verdict == models.ScanVerdictDrainer && req.HackerAddress != "" {
		reason := "transferred funds to known drainer operator " + req.HackerAddress
		seen := map[string]bool{req.HackerAddress: true}
		for _, addr := range req.ExposedAddresses {
			if addr == "" || seen[addr] {
				continue
			}
			seen[addr] = true
			if err := h.repo.MarkAssociatedHacker(c.Request.Context(), addr, req.Chain, reason); err != nil {
				log.Printf("scanner: failed to mark association %s: %v", addr, err)
				continue
			}
			associated++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           id,
		"inserted":     inserted,
		"victim_added": victimAdded,
		"hacker_added": hackerAdded,
		"associated":   associated,
	})
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

// GetMonitorStats returns aggregate counters for the live monitoring page.
func (h *Handler) GetMonitorStats(c *gin.Context) {
	stats, err := h.repo.GetScanStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "database error",
			Code:  "DB_ERROR",
		})
		return
	}
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
}

// buildStatusEvidence composes the chain of evidence explaining the wallet
// status: registry listing, leaked key material and scanner indicators with
// the transactions they were detected in.
func (h *Handler) buildStatusEvidence(c *gin.Context, report *models.ReportResponse) []models.StatusEvidence {
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

	findings, err := h.repo.GetScanFindingsForAddress(c.Request.Context(), report.Address, 20)
	if err != nil {
		log.Printf("report evidence: failed to load scan findings for %s: %v", report.Address, err)
		return evidence
	}
	for _, f := range findings {
		role := "victim"
		counterparty := f.HackerAddress
		if f.HackerAddress == report.Address {
			role = "hacker"
			counterparty = f.VictimAddress
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
				AmountSOL:    f.AmountSOL,
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
