package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/models"
	"vauln-address/internal/validators"
)

// bulkCheckMaxAddresses caps one bulk lookup: the live-block scanners batch
// their candidates below this limit, and the cap keeps a single request
// from turning into an unbounded IN query.
const bulkCheckMaxAddresses = 500

// bulkCheckReportedStatuses is the threat subset the bulk endpoint reports:
// the live-block scanners only track movements of these statuses, so the
// endpoint does not disclose the rest of the registry either.
var bulkCheckReportedStatuses = map[models.WalletStatus]bool{
	models.StatusHacked:     true,
	models.StatusHacker:     true,
	models.StatusDrained:    true,
	models.StatusPhishing:   true,
	models.StatusSuspicious: true,
}

// BulkCheckWallets looks up a batch of addresses of one chain in the threat
// database (POST /api/check/bulk). Only addresses present in the database
// with a reported threat status (hacked, hacker, drained, phishing,
// suspicious) are returned, with their status and registry metadata;
// unknown or malformed addresses are silently skipped (reflected in
// checked/found). Specific EVM network names (bnb, base, …) are accepted
// and normalized to the canonical "evm" chain, exactly like scanner ingest.
func (h *Handler) BulkCheckWallets(c *gin.Context) {
	var req models.BulkCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	req.Chain = strings.ToLower(strings.TrimSpace(req.Chain))
	if !models.IsScanChain(req.Chain) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "unsupported chain",
			Code:    "INVALID_CHAIN",
			Details: "supported chains: evm, btc, solana, sui, tron and the EVM networks (ethereum, bnb, base, linea, arbitrum, polygon, optimism, avalanche)",
		})
		return
	}
	chain := models.CanonicalChain(req.Chain)

	if len(req.Addresses) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "addresses must not be empty",
			Code:  "EMPTY_ADDRESSES",
		})
		return
	}
	if len(req.Addresses) > bulkCheckMaxAddresses {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "too many addresses",
			Code:    "TOO_MANY_ADDRESSES",
			Details: "the batch limit is 500 addresses per request",
		})
		return
	}

	// Dedupe and drop malformed addresses: the DB lookup is exact, so
	// invalid input can never match anyway.
	valid := make([]string, 0, len(req.Addresses))
	seen := make(map[string]bool, len(req.Addresses))
	for _, addr := range req.Addresses {
		addr = strings.TrimSpace(addr)
		if addr == "" || seen[addr] {
			continue
		}
		seen[addr] = true
		if ok, _ := validators.ValidateAddress(chain, addr); !ok {
			continue
		}
		valid = append(valid, addr)
	}

	found, err := h.repo.GetWalletsByAddresses(c.Request.Context(), chain, valid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "database error",
			Code:  "DB_ERROR",
		})
		return
	}

	// Answer in the request order, not the database scan order.
	byAddr := make(map[string]models.BulkCheckResult, len(found))
	for _, res := range found {
		if !bulkCheckReportedStatuses[models.WalletStatus(res.Status)] {
			continue
		}
		key := res.Address
		if chain == string(models.ChainEVM) {
			key = strings.ToLower(key)
		}
		byAddr[key] = res
	}
	results := make([]models.BulkCheckResult, 0, len(found))
	for _, addr := range valid {
		key := addr
		if chain == string(models.ChainEVM) {
			key = strings.ToLower(key)
		}
		if res, ok := byAddr[key]; ok {
			results = append(results, res)
		}
	}

	c.JSON(http.StatusOK, models.BulkCheckResponse{
		Chain:   chain,
		Checked: len(valid),
		Found:   len(results),
		Results: results,
	})
}
