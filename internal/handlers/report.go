package handlers

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/models"
	"vauln-address/internal/repository"
	"vauln-address/internal/validators"
)

const (
	reportTreeDepth       = 2
	reportTreeMaxNodes    = 48
	reportTreeMaxChildren = 20
	reportTreeFindingsCap = 250
)

var chainCurrency = map[string]string{
	"evm":       "ETH",
	"btc":       "BTC",
	"solana":    "SOL",
	"sui":       "SUI",
	"tron":      "TRX",
	"ethereum":  "ETH",
	"bnb":       "BNB",
	"base":      "ETH",
	"linea":     "ETH",
	"arbitrum":  "ETH",
	"polygon":   "POL",
	"optimism":  "ETH",
	"avalanche": "AVAX",
}

// GetReport returns a detailed report for an address found in the database.
// Access requires a previous check of this address by the same requester:
// the authenticated wallet (kept forever) or the same IP within
// models.AnonymousReportTTL for anonymous users.
func (h *Handler) GetReport(c *gin.Context) {
	address := strings.TrimSpace(c.Query("address"))
	chain := strings.ToLower(strings.TrimSpace(c.Query("chain")))

	if address == "" || chain == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "address and chain are required",
			Code:  "INVALID_REQUEST",
		})
		return
	}
	if !models.IsValidChain(chain) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "unsupported chain",
			Code:  "INVALID_CHAIN",
		})
		return
	}
	if valid, errMsg := validators.ValidateAddress(chain, address); !valid {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid address format",
			Code:    "INVALID_ADDRESS",
			Details: errMsg,
		})
		return
	}

	requester, anonymous := reportRequester(c)

	lastCheck, err := h.repo.GetLastReportAccess(c.Request.Context(), requester, address, chain)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "database error",
			Code:  "DB_ERROR",
		})
		return
	}

	var expiresAt *time.Time
	switch {
	case lastCheck == nil:
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "report is available only after you check this address",
			Code:    "REPORT_NOT_AVAILABLE",
			Details: "run a check on the main page first",
		})
		return
	case anonymous && time.Since(*lastCheck) > models.AnonymousReportTTL:
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "report expired",
			Code:    "REPORT_EXPIRED",
			Details: "reports of anonymous users are deleted 24 hours after the check; authorize to keep them",
		})
		return
	case anonymous:
		exp := lastCheck.Add(models.AnonymousReportTTL)
		expiresAt = &exp
	}

	report, statusCode := h.assembleReport(c, address, chain)
	if report == nil {
		c.JSON(statusCode, models.ErrorResponse{
			Error: "address not found in database",
			Code:  "NOT_FOUND",
		})
		return
	}
	report.ExpiresAt = expiresAt

	c.JSON(http.StatusOK, report)
}

// assembleReport builds the full report payload for an address that passed
// access control: wallet row, leaks, details and the transaction tree.
// Returns (nil, statusCode) when the address cannot be reported.
func (h *Handler) assembleReport(c *gin.Context, address, chain string) (*models.ReportResponse, int) {
	report, err := h.repo.GetWalletReport(c.Request.Context(), address, chain)
	if err != nil {
		return nil, http.StatusInternalServerError
	}
	if report == nil {
		return nil, http.StatusNotFound
	}

	leaks, err := h.repo.GetLeaksForAddress(c.Request.Context(), address, chain)
	if err != nil {
		return nil, http.StatusInternalServerError
	}
	report.Leaks = leaks
	report.Details = buildReportDetails(report, leaks)
	report.Evidence = h.buildStatusEvidence(c, report)
	report.Transactions = h.buildTxTree(c, address, chain)
	report.FundFlows = h.buildFundFlows(c, address, chain)
	return report, http.StatusOK
}

// reportRequester identifies who asks for the report: the authenticated
// wallet address, or "ip:<ip>" for anonymous users. The value matches what
// CheckWallet stores in check_history.wallet_address.
func reportRequester(c *gin.Context) (string, bool) {
	if ua, exists := c.Get("userAddress"); exists {
		if addr, ok := ua.(string); ok && addr != "" {
			return addr, false
		}
	}
	return repository.AnonymousRequesterPrefix + c.ClientIP(), true
}

// buildReportDetails composes the human-readable explanation shown on the
// report page. The primary reason comes from the existing wallets.reason
// column; the details describe what the status and leaked data mean.
func buildReportDetails(report *models.ReportResponse, leaks []models.LeakedKeyInfo) string {
	var exposed []string
	if report.HasPK {
		exposed = append(exposed, "private key")
	}
	if report.HasSeed {
		exposed = append(exposed, "mnemonic (seed) phrase")
	}
	for _, leak := range leaks {
		if leak.KeyType == "private_key" && !report.HasPK {
			exposed = append(exposed, "private key")
		}
		if leak.KeyType == "seed" && !report.HasSeed {
			exposed = append(exposed, "mnemonic (seed) phrase")
		}
	}
	exposedList := strings.Join(exposed, " and ")

	if report.Status == string(models.StatusHacked) && exposedList != "" {
		return fmt.Sprintf("The %s of this wallet is publicly available to everyone. Anyone can import the wallet and steal all funds. Do not use this address.", exposedList)
	}
	if desc, ok := models.StatusDescription(models.WalletStatus(report.Status)); ok {
		return desc
	}
	return fmt.Sprintf("Wallet status: %s.", report.Status)
}

// ==================== Transaction tree ====================

// buildTxTree builds the transaction tree from real indexed scanner data:
// child nodes are counterparties of the address in scan_findings, aggregated
// by address (tx count = number of findings, amount = total SOL moved).
// Counterparties that are on-chain programs invoked by the transaction are
// marked as programs and never expanded; hacker wallets are the sink of the
// stolen funds, so the tree stops at them instead of following the funds
// further. Addresses without indexed findings produce an empty tree.
func (h *Handler) buildTxTree(c *gin.Context, address, chain string) *models.ReportTxNode {
	root := &models.ReportTxNode{Address: address, Currency: currencyOf(chain)}
	root.Status = h.treeNodeStatus(c, address, chain)
	nodes := 1
	visited := map[string]bool{address: true}
	h.fillTxNode(c, root, chain, 0, visited, &nodes)
	return root
}

func (h *Handler) fillTxNode(c *gin.Context, node *models.ReportTxNode, chain string, depth int, visited map[string]bool, nodes *int) {
	findings, err := h.repo.GetScanFindingsForAddress(c.Request.Context(), node.Address, reportTreeFindingsCap)
	if err != nil {
		return
	}

	type counterparty struct {
		address    string
		chain      string
		txCount    int
		amount     float64
		isProgram  bool
		payout     bool // wallet received payouts FROM this node (fund flow)
		repeat     bool // recurring payouts (2+ transfers) => hacker
		signatures []string
	}
	byAddr := map[string]*counterparty{}
	total := 0.0
	for _, f := range findings {
		total += f.AmountSOL
		other := scanCounterparty(node.Address, f)
		if other == "" || other == node.Address || visited[other] {
			continue
		}
		// показываем в дереве только валидные адреса: плейсхолдеры
		// прошлых прогонов сканера не должны попадать в отчёт
		if ok, _ := validators.ValidateAddress(treeValidationChain(f.Chain, chain), other); !ok {
			continue
		}
		cp := byAddr[other]
		if cp == nil {
			cp = &counterparty{address: other, chain: f.Chain}
			byAddr[other] = cp
		}
		cp.txCount++
		cp.amount += f.AmountSOL
		if f.Signature != "" {
			cp.signatures = append(cp.signatures, f.Signature)
		}
		// A takeover-only finding names the owning program as the hacker:
		// the assign instruction never invokes the owner program, so older
		// findings don't list it under programs.
		if isProgramAddress(other, f.Programs) ||
			(other == f.HackerAddress && isTakeoverOnlyFinding(f.Indicators)) {
			cp.isProgram = true
		}
	}
	node.TxCount = len(findings)
	node.Amount = roundAmount(total)

	if depth >= reportTreeDepth || *nodes >= reportTreeMaxNodes {
		return
	}

	// The tree stops at hacker wallets: they are the sink of the stolen
	// funds. Programs are NOT a sink — the funds only pass through them, so
	// a program node is expanded to the real payout recipients below.
	if depth > 0 && node.Status == string(models.StatusHacker) && !node.IsProgram {
		return
	}

	// Payout recipients: flow-trace findings (F1/F2) record which operator
	// wallets funded each downstream address — this links every wallet that
	// received payouts from this node back to it.
	payouts, err := h.repo.GetFlowPayoutsForSource(c.Request.Context(), node.Address, reportTreeFindingsCap)
	if err == nil {
		for _, f := range payouts {
			recipient := f.HackerAddress
			if recipient == "" || recipient == node.Address || visited[recipient] || byAddr[recipient] != nil {
				continue
			}
			if ok, _ := validators.ValidateAddress(treeValidationChain(f.Chain, chain), recipient); !ok {
				continue
			}
			cp := &counterparty{address: recipient, chain: f.Chain, payout: true}
			byAddr[recipient] = cp
		}
		// Recurrence decides the verdict: a wallet paid repeatedly (2+
		// transfers, or seen in several findings) is a hacker, a one-off
		// recipient stays suspicious.
		counts := map[string]int{}
		for _, f := range payouts {
			counts[f.HackerAddress]++
		}
		for _, f := range payouts {
			cp := byAddr[f.HackerAddress]
			if cp == nil || !cp.payout {
				continue
			}
			cp.txCount++
			cp.amount += f.AmountSOL
			if f.Signature != "" {
				cp.signatures = append(cp.signatures, f.Signature)
			}
			if counts[f.HackerAddress] >= 2 || hasIndicator(f.Indicators, "F2_REPEAT_DOWNSTREAM") {
				cp.repeat = true
			}
		}
	}

	list := make([]*counterparty, 0, len(byAddr))
	for _, cp := range byAddr {
		// A program node shows only where the funds went afterwards: the
		// victims that were drained through it are not payout recipients.
		if node.IsProgram && !cp.payout {
			continue
		}
		list = append(list, cp)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].amount != list[j].amount {
			return list[i].amount > list[j].amount
		}
		return list[i].address < list[j].address
	})

	for _, cp := range list {
		if len(node.Children) >= reportTreeMaxChildren || *nodes >= reportTreeMaxNodes {
			break
		}
		visited[cp.address] = true
		*nodes = *nodes + 1
		child := &models.ReportTxNode{
			Address:          cp.address,
			Currency:         node.Currency,
			Status:           h.treeNodeStatus(c, cp.address, cp.chain),
			AssociatedHacker: h.repo.GetWalletAssociation(c.Request.Context(), cp.address, cp.chain),
			IsProgram:        cp.isProgram,
			Signatures:       cp.signatures,
		}
		if cp.isProgram {
			child.Status = models.TreeStatusProgram
		} else if cp.payout && child.Status == models.TreeStatusUnknown {
			// Not registered yet: classify by payout recurrence.
			if cp.repeat {
				child.Status = string(models.StatusHacker)
			} else {
				child.Status = string(models.StatusSuspicious)
			}
		}
		node.Children = append(node.Children, child)
		h.fillTxNode(c, child, chain, depth+1, visited, nodes)
	}
}

// isProgramAddress reports whether addr is one of the on-chain programs the
// scanner recorded for the finding. A program id is never the hacker wallet:
// the funds only passed through it.
func isProgramAddress(addr string, programs []string) bool {
	for _, p := range programs {
		if p == addr {
			return true
		}
	}
	return false
}

// scanCounterparty returns the other party of a scan finding relative to
// address: the hacker when address is the victim and vice versa.
func scanCounterparty(address string, f models.ScanFinding) string {
	switch {
	case f.VictimAddress == address:
		return f.HackerAddress
	case f.HackerAddress == address:
		return f.VictimAddress
	}
	return ""
}

// buildFundFlows aggregates the directional money-flow analytics of a
// report: per counterparty — how much this wallet received from / sent to
// it, how many findings link them, and the exact transaction signatures.
// Returns nil when nothing is indexed for the address.
func (h *Handler) buildFundFlows(c *gin.Context, address, chain string) *models.ReportFundFlows {
	findings, err := h.repo.GetScanFindingsForAddress(c.Request.Context(), address, reportTreeFindingsCap)
	if err != nil {
		return nil
	}
	payouts, err := h.repo.GetFlowPayoutsForSource(c.Request.Context(), address, reportTreeFindingsCap)
	if err != nil {
		payouts = nil
	}
	if len(findings) == 0 && len(payouts) == 0 {
		return nil
	}

	type agg struct {
		entry    models.ReportFlowEntry
		programs bool
	}
	out := map[string]*agg{}
	in := map[string]*agg{}

	add := func(dst map[string]*agg, addr, fchain string, f models.ScanFinding) {
		if addr == "" || addr == address {
			return
		}
		// плейсхолдеры прошлых прогонов сканера не должны попадать в отчёт
		if ok, _ := validators.ValidateAddress(treeValidationChain(fchain, chain), addr); !ok {
			return
		}
		a := dst[addr]
		if a == nil {
			a = &agg{entry: models.ReportFlowEntry{Address: addr, Chain: fchain}}
			dst[addr] = a
		}
		a.entry.TxCount++
		a.entry.Amount += f.AmountSOL
		if f.Signature != "" {
			a.entry.Signatures = append(a.entry.Signatures, f.Signature)
		}
		if isProgramAddress(addr, f.Programs) ||
			(addr == f.HackerAddress && isTakeoverOnlyFinding(f.Indicators)) {
			a.programs = true
		}
	}

	for _, f := range findings {
		switch {
		case f.VictimAddress == address:
			// this wallet was drained: funds went OUT to the counterparty
			add(out, f.HackerAddress, f.Chain, f)
		case f.HackerAddress == address:
			// this wallet is the operator: funds came IN from the victim
			add(in, f.VictimAddress, f.Chain, f)
		}
	}
	for _, f := range payouts {
		// flow-trace findings record payouts FROM this wallet downstream
		add(out, f.HackerAddress, f.Chain, f)
	}

	flows := &models.ReportFundFlows{Currency: currencyOf(chain)}
	flatten := func(m map[string]*agg) []models.ReportFlowEntry {
		list := make([]models.ReportFlowEntry, 0, len(m))
		for _, a := range m {
			// Programs are not money recipients: a takeover assigns the
			// account to the drainer program, but the funds go to real
			// wallets. Showing the program here hides the actual recipient.
			if a.programs {
				continue
			}
			a.entry.Amount = roundAmount(a.entry.Amount)
			a.entry.Status = h.treeNodeStatus(c, a.entry.Address, a.entry.Chain)
			a.entry.AssociatedHacker = h.repo.GetWalletAssociation(c.Request.Context(), a.entry.Address, a.entry.Chain)
			list = append(list, a.entry)
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Amount != list[j].Amount {
				return list[i].Amount > list[j].Amount
			}
			return list[i].Address < list[j].Address
		})
		return list
	}
	flows.Inflow = flatten(in)
	flows.Outflow = flatten(out)
	return flows
}

// treeNodeStatus resolves a tree address against the wallets table;
// addresses not present in the database are reported as unknown.
func (h *Handler) treeNodeStatus(c *gin.Context, address, chain string) string {
	status, err := h.repo.GetWalletStatus(c.Request.Context(), address, chain)
	if err == nil && status != "" {
		return status
	}
	return models.TreeStatusUnknown
}

// treeValidationChain picks the chain used to validate counterparties in
// the transaction tree: the counterparty address is validated against the
// chain recorded in its finding (scan findings always carry their own
// chain), with the report chain as fallback.
func treeValidationChain(findingChain, reportChain string) string {
	// Specific EVM networks (bnb, base, …) share the EVM address format.
	findingChain = models.CanonicalChain(findingChain)
	if models.IsValidChain(findingChain) {
		return findingChain
	}
	return reportChain
}

func currencyOf(chain string) string {
	if cur, ok := chainCurrency[chain]; ok {
		return cur
	}
	return strings.ToUpper(chain)
}

func roundAmount(v float64) float64 {
	return math.Round(v*10000) / 10000
}
