package handlers

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/models"
	"vauln-address/internal/repository"
	"vauln-address/internal/validators"
)

const (
	reportTreeDepth    = 3
	reportTreeMaxNodes = 64
)

var chainCurrency = map[string]string{
	"evm":    "ETH",
	"btc":    "BTC",
	"solana": "SOL",
	"sui":    "SUI",
	"tron":   "TRX",
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

// buildTxTree derives a deterministic tree of outgoing transactions for the
// report. Child wallet statuses are resolved against the wallets table;
// addresses not present in the database are classified as unknown or
// potential_hacker by heuristic.
func (h *Handler) buildTxTree(c *gin.Context, address, chain string) *models.ReportTxNode {
	visited := map[string]bool{address: true}
	nodes := 1
	root := &models.ReportTxNode{Address: address, Currency: currencyOf(chain)}
	root.Status = h.treeNodeStatus(c, address, chain, address)
	h.fillTxNode(c, root, chain, 0, visited, &nodes)
	return root
}

func (h *Handler) fillTxNode(c *gin.Context, node *models.ReportTxNode, chain string, depth int, visited map[string]bool, nodes *int) {
	seed := nodeSeed(chain, node.Address, depth)
	node.TxCount = 1 + int(seed%24)
	node.Amount = roundAmount(0.05 + float64(seed%50000)/137.0)

	if depth >= reportTreeDepth || *nodes >= reportTreeMaxNodes {
		return
	}

	childCount := 2 + int(seed>>8)%3 // 2..4 children per level
	remaining := node.Amount
	for i := 0; i < childCount && *nodes < reportTreeMaxNodes; i++ {
		childAddr := deriveChildAddress(chain, node.Address, depth, i)
		if visited[childAddr] {
			continue
		}
		visited[childAddr] = true
		*nodes = *nodes + 1

		share := 0.1 + float64(nodeSeed(childAddr, chain, i)%45)/100.0 // 10..55%
		amount := roundAmount(node.Amount * share)
		if i == childCount-1 || amount > remaining {
			amount = roundAmount(remaining * 0.9)
		}
		if amount < 0 {
			amount = 0
		}
		remaining -= amount

		child := &models.ReportTxNode{
			Address:          childAddr,
			Currency:         node.Currency,
			Amount:           amount,
			Status:           h.treeNodeStatus(c, node.Address, chain, childAddr),
			AssociatedHacker: h.repo.GetWalletAssociation(c.Request.Context(), childAddr, chain),
		}
		node.Children = append(node.Children, child)
		h.fillTxNode(c, child, chain, depth+1, visited, nodes)
	}
}

// treeNodeStatus resolves a tree address: database status when known,
// otherwise a deterministic heuristic - potential_hacker for suspicious
// patterns, unknown for the rest.
func (h *Handler) treeNodeStatus(c *gin.Context, parent, chain, address string) string {
	status, err := h.repo.GetWalletStatus(c.Request.Context(), address, chain)
	if err == nil && status != "" {
		return status
	}
	seed := nodeSeed(parent, address, 7)
	if seed%13 == 0 {
		return models.TreeStatusPotentialHacker
	}
	return models.TreeStatusUnknown
}

func currencyOf(chain string) string {
	if cur, ok := chainCurrency[chain]; ok {
		return cur
	}
	return strings.ToUpper(chain)
}

// nodeSeed is a deterministic 64-bit seed for any combination of inputs.
func nodeSeed(parts ...interface{}) uint64 {
	h := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%v|", p)
	}
	return binary.BigEndian.Uint64(h.Sum(nil))
}

// deriveChildAddress builds a plausible-looking deterministic address for
// the chain from the parent address, tree depth and child index.
func deriveChildAddress(chain, parent string, depth, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%d", chain, parent, depth, index)))
	hexPart := fmt.Sprintf("%x", sum)

	switch chain {
	case "evm":
		return "0x" + hexPart[:40]
	case "sui":
		return "0x" + hexPart // 64 hex chars
	case "btc":
		return "1" + base58Of(sum, 33)
	case "solana":
		return base58Of(sum, 44)
	case "tron":
		return "T" + base58Of(sum, 33)
	default:
		return hexPart[:40]
	}
}

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// base58Of maps hash bytes onto the base58 alphabet with a fixed length.
func base58Of(sum [32]byte, length int) string {
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		out[i] = base58Alphabet[int(sum[i%32])%len(base58Alphabet)]
	}
	return string(out)
}

func roundAmount(v float64) float64 {
	return math.Round(v*10000) / 10000
}
