package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/models"
	"vauln-address/internal/repository"
	"vauln-address/internal/validators"
)

type Handler struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "vauln-address-api",
	})
}

func (h *Handler) CheckWallet(c *gin.Context) {
	var req models.CheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	req.Address = strings.TrimSpace(req.Address)
	req.Chain = strings.ToLower(strings.TrimSpace(req.Chain))

	if !models.IsValidChain(req.Chain) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "unsupported chain",
			Code:    "INVALID_CHAIN",
			Details: "supported chains: evm, btc, solana, sui, tron",
		})
		return
	}

	valid, errMsg := validators.ValidateAddress(req.Chain, req.Address)
	if !valid {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid address format",
			Code:    "INVALID_ADDRESS",
			Details: errMsg,
		})
		return
	}

	wallet, err := h.repo.GetWallet(c.Request.Context(), req.Address, req.Chain)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "database error",
			Code:  "DB_ERROR",
		})
		return
	}

	response := models.CheckResponse{
		Address: req.Address,
		Chain:   req.Chain,
		Found:   wallet != nil,
	}

	if wallet != nil {
		response.Status = string(wallet.Status)
		response.HasPK = wallet.HasPK
		response.HasSeed = wallet.HasSeed
	} else {
		response.Status = "not_found"
	}

	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetRecentChecks(c *gin.Context) {
	checks, err := h.repo.GetRecentChecks(c.Request.Context(), 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "database error",
			Code:  "DB_ERROR",
		})
		return
	}

	if checks == nil {
		checks = []models.RecentCheck{}
	}

	c.JSON(http.StatusOK, gin.H{
		"checks": checks,
		"count":  len(checks),
	})
}

func (h *Handler) SubmitContact(c *gin.Context) {
	var req models.ContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	msg := &models.ContactMessage{
		Name:    strings.TrimSpace(req.Name),
		Email:   strings.TrimSpace(req.Email),
		Message: strings.TrimSpace(req.Message),
	}

	if err := h.repo.SaveContactMessage(c.Request.Context(), msg); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "failed to save message",
			Code:  "DB_ERROR",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "contact form submitted successfully",
	})
}

func (h *Handler) GetSupportedChains(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"chains": []gin.H{
			{"name": "EVM", "id": "evm", "example": "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1"},
			{"name": "Bitcoin", "id": "btc", "example": "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"},
			{"name": "Solana", "id": "solana", "example": "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV"},
			{"name": "Sui", "id": "sui", "example": "0x8a角1c4cd2d2fd05e02e3d2b5f4d6f8a1c3e5b7d9f0a2c4e6"},
			{"name": "Tron", "id": "tron", "example": "TJK5M5kKxP8xF9cGvN2pL6rU4sW7xA3bCd"},
		},
	})
}
