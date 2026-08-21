package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/auth"
	"vauln-address/internal/models"
	"vauln-address/internal/validators"
)

// Public share links carry a token of 16 bytes formatted as a UUID:
// the first 8 bytes are the check_history row id, the last 8 bytes are a
// truncated HMAC over it. Whoever holds the link can open the report;
// only authenticated users can mint a token, so anonymous users cannot
// make their reports public. No extra schema is needed: the token
// resolves through the existing check_history row.
const shareTokenPurpose = "report-share-v1"

func makeShareToken(checkID int64) string {
	mac := hmac.New(sha256.New, []byte(auth.JWTSecret))
	fmt.Fprintf(mac, "%s|%d", shareTokenPurpose, checkID)
	sig := mac.Sum(nil)

	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(checkID))
	copy(b[8:], sig[:8])
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// parseShareToken validates the token and returns the check row id.
func parseShareToken(token string) (int64, bool) {
	compact := strings.ReplaceAll(token, "-", "")
	if len(compact) != 32 {
		return 0, false
	}
	raw, err := hex.DecodeString(compact)
	if err != nil || len(raw) != 16 {
		return 0, false
	}

	checkID := int64(binary.BigEndian.Uint64(raw[:8]))
	if makeShareToken(checkID) != strings.ToLower(token) {
		return 0, false
	}
	return checkID, true
}

// ShareReport mints a public share link for an address the authenticated
// user has checked. Anonymous users cannot share reports.
func (h *Handler) ShareReport(c *gin.Context) {
	var req models.ShareReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	address := strings.TrimSpace(req.Address)
	chain := strings.ToLower(strings.TrimSpace(req.Chain))
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

	requester, _ := reportRequester(c)
	checkID, _, found, err := h.repo.GetReportCheckRow(c.Request.Context(), requester, address, chain)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "database error", Code: "DB_ERROR"})
		return
	}
	if !found {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "report is available only after you check this address",
			Code:    "REPORT_NOT_AVAILABLE",
			Details: "run a check on the main page first",
		})
		return
	}

	report, err := h.repo.GetWalletReport(c.Request.Context(), address, chain)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "database error", Code: "DB_ERROR"})
		return
	}
	if report == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "address not found in database",
			Code:  "NOT_FOUND",
		})
		return
	}

	shareID := makeShareToken(checkID)
	c.JSON(http.StatusOK, models.ShareReportResponse{
		ShareID:  shareID,
		ShareURL: "/report/" + shareID,
	})
}

// GetSharedReport serves a publicly shared report by its UUID token.
// No authentication and no 24h expiry: the link itself is the capability.
func (h *Handler) GetSharedReport(c *gin.Context) {
	checkID, ok := parseShareToken(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "invalid share link",
			Code:  "INVALID_SHARE",
		})
		return
	}

	address, chain, err := h.repo.GetReportCheckByID(c.Request.Context(), checkID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "shared report no longer exists",
			Code:  "INVALID_SHARE",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "database error", Code: "DB_ERROR"})
		return
	}

	report, statusCode := h.assembleReport(c, address, chain)
	if report == nil {
		c.JSON(statusCode, models.ErrorResponse{
			Error: "address not found in database",
			Code:  "NOT_FOUND",
		})
		return
	}
	report.Public = true
	c.JSON(http.StatusOK, report)
}
