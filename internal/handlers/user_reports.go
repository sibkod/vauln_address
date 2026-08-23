package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/models"
	"vauln-address/internal/repository"
	"vauln-address/internal/services"
)

// submitReportRequester resolves the reporter identity: the authenticated
// wallet when present, otherwise the caller IP (anonymous).
func submitReportRequester(c *gin.Context) string {
	if ua, exists := c.Get("userAddress"); exists {
		if addr, ok := ua.(string); ok && addr != "" {
			return addr
		}
	}
	return repository.AnonymousRequesterPrefix + c.ClientIP()
}

func verifyReportCaptcha(c *gin.Context, h *Handler, id, answer string) bool {
	if h.captcha.Verify(id, answer) {
		return true
	}
	c.JSON(http.StatusForbidden, models.ErrorResponse{
		Error:   "captcha verification failed",
		Code:    "CAPTCHA_INVALID",
		Details: "request a new captcha and enter the code from the image",
	})
	return false
}

// SubmitBugReport accepts a "report an error" message from a report page.
// The captcha is verified before anything is stored or forwarded.
func (h *Handler) SubmitBugReport(c *gin.Context) {
	var req models.BugReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}
	if !verifyReportCaptcha(c, h, req.CaptchaID, req.CaptchaAnswer) {
		return
	}

	rep := &models.BugReport{
		Address:  strings.TrimSpace(req.Address),
		Chain:    strings.ToLower(strings.TrimSpace(req.Chain)),
		Message:  strings.TrimSpace(req.Message),
		Reporter: submitReportRequester(c),
		Status:   "new",
	}

	id, err := h.repo.InsertBugReport(c.Request.Context(), rep)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "database error",
			Code:  "DB_ERROR",
		})
		return
	}
	rep.ID = id

	sent := h.forwardBugReportToTelegram(rep)

	c.JSON(http.StatusOK, gin.H{
		"id":            id,
		"status":        rep.Status,
		"telegram_sent": sent,
		"message":       "report submitted, thank you for the heads-up",
	})
}

func (h *Handler) forwardBugReportToTelegram(rep *models.BugReport) bool {
	reportCtx := "—"
	if rep.Address != "" {
		reportCtx = fmt.Sprintf("<code>%s</code> (%s)", html.EscapeString(rep.Address), html.EscapeString(rep.Chain))
	}
	text := fmt.Sprintf(
		"🐞 <b>Bug report #%d</b>\n"+
			"Report context: %s\n"+
			"Message: %s\n"+
			"Reporter: <code>%s</code>",
		rep.ID,
		reportCtx,
		html.EscapeString(rep.Message),
		html.EscapeString(rep.Reporter),
	)
	if err := h.telegram.Send(context.Background(), text); err != nil {
		if err == services.ErrTelegramNotConfigured {
			log.Printf("bug report #%d: telegram bot not configured, stored only", rep.ID)
		} else {
			log.Printf("bug report #%d: telegram send failed: %v", rep.ID, err)
		}
		return false
	}
	if err := h.repo.MarkBugReportTelegram(context.Background(), rep.ID, true); err != nil {
		log.Printf("bug report #%d: failed to mark telegram_sent: %v", rep.ID, err)
	}
	return true
}

// SubmitLeakReport accepts a leaked private key or seed phrase. The secret
// is forwarded to the team chat and only its SHA-256 fingerprint is stored
// (for dedup/audit) — the secret itself never touches the database.
func (h *Handler) SubmitLeakReport(c *gin.Context) {
	var req models.LeakReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}
	if !verifyReportCaptcha(c, h, req.CaptchaID, req.CaptchaAnswer) {
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

	secret := strings.TrimSpace(req.Secret)
	sum := sha256.Sum256([]byte(secret))
	rep := &models.LeakReport{
		Chain:       chain,
		SecretType:  req.SecretType,
		SecretHash:  hex.EncodeToString(sum[:]),
		Description: strings.TrimSpace(req.Description),
		Reporter:    submitReportRequester(c),
		Status:      "new",
	}

	id, err := h.repo.InsertLeakReport(c.Request.Context(), rep)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "database error",
			Code:  "DB_ERROR",
		})
		return
	}
	rep.ID = id

	sent := h.forwardLeakReportToTelegram(rep, secret)

	c.JSON(http.StatusOK, gin.H{
		"id":            id,
		"status":        rep.Status,
		"telegram_sent": sent,
		"message":       "leak submitted, our analysts will review it",
	})
}

func (h *Handler) forwardLeakReportToTelegram(rep *models.LeakReport, secret string) bool {
	typeLabel := "private key"
	if rep.SecretType == "seed_phrase" {
		typeLabel = "seed phrase"
	}
	text := fmt.Sprintf(
		"🔑 <b>New leak report #%d</b>\n"+
			"Type: <b>%s</b> (chain %s)\n"+
			"Secret: <code>%s</code>\n"+
			"Description: %s\n"+
			"Reporter: <code>%s</code>",
		rep.ID,
		typeLabel,
		html.EscapeString(strings.ToUpper(rep.Chain)),
		html.EscapeString(secret),
		html.EscapeString(orDash(rep.Description)),
		html.EscapeString(rep.Reporter),
	)
	if err := h.telegram.Send(context.Background(), text); err != nil {
		if err == services.ErrTelegramNotConfigured {
			log.Printf("leak report #%d: telegram bot not configured, stored only", rep.ID)
		} else {
			log.Printf("leak report #%d: telegram send failed: %v", rep.ID, err)
		}
		return false
	}
	if err := h.repo.MarkLeakReportTelegram(context.Background(), rep.ID, true); err != nil {
		log.Printf("leak report #%d: failed to mark telegram_sent: %v", rep.ID, err)
	}
	return true
}
