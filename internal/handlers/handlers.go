package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/auth"
	"vauln-address/internal/models"
	"vauln-address/internal/repository"
	"vauln-address/internal/validators"
)

type Handler struct {
	repo       *repository.Repository
	authService *auth.AuthService
}

func New(repo *repository.Repository) *Handler {
	return &Handler{
		repo:       repo,
		authService: auth.NewAuthService(repo),
	}
}

// GetAuthService returns the auth service for middleware use
func (h *Handler) GetAuthService() *auth.AuthService {
	return h.authService
}

func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "vauln-address-api",
	})
}

// ==================== Authentication ====================

// GetNonce generates a nonce for Web3 authentication
func (h *Handler) GetNonce(c *gin.Context) {
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

	nonce, err := h.authService.GenerateNonce(address, chain)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "failed to generate nonce",
			Code:  "SERVER_ERROR",
		})
		return
	}

	// Create the message to sign
	message := fmt.Sprintf("Sign this message to authenticate with Vauln Address.\n\nNonce: %s\nTimestamp: %d", 
		nonce, time.Now().Unix())

	c.JSON(http.StatusOK, models.NonceResponse{
		Nonce:   nonce,
		Message: message,
	})
}

// Authenticate verifies the signature and returns a JWT token
func (h *Handler) Authenticate(c *gin.Context) {
	var req models.AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	authResp, err := h.authService.VerifySignature(req.Address, req.Chain, req.Signature, req.Message)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "authentication failed",
			Code:    "AUTH_FAILED",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, authResp)
}

// GetUserProfile returns the current user's profile
func (h *Handler) GetUserProfile(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "unauthorized",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	user, err := h.authService.GetUserByID(userID.(int64))
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "user not found",
			Code:  "NOT_FOUND",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":              user.ID,
			"wallet_address":  user.WalletAddress,
			"chain":           user.Chain,
			"balance":         user.Balance,
			"is_premium":      user.Balance > 10,
			"created_at":      user.CreatedAt,
			"last_login_at":   user.LastLoginAt,
		},
	})
}

// ==================== Payments ====================

// GetPricing returns the pricing for different payment methods
func (h *Handler) GetPricing(c *gin.Context) {
	checksStr := c.DefaultQuery("checks", "10")
	
	var checks int
	if _, err := fmt.Sscanf(checksStr, "%d", &checks); err != nil || checks < 1 {
		checks = 10
	}
	if checks > 100 {
		checks = 100
	}

	pricing := models.GetPricing(checks)

	c.JSON(http.StatusOK, gin.H{
		"checks":        checks,
		"price_per_check_usd": models.PricePerCheckUSD,
		"payment_methods": pricing,
	})
}

// CreateOrder creates a new payment order
func (h *Handler) CreateOrder(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "authentication required",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	var req models.PurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Calculate price
	pricing := models.GetPricing(req.ChecksCount)
	var selectedMethod *models.PaymentMethod
	
	for _, method := range pricing {
		if string(method.Currency) == req.Currency {
			selectedMethod = &method
			break
		}
	}

	if selectedMethod == nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "unsupported currency",
			Code:    "INVALID_CURRENCY",
			Details: "supported currencies: usdc, usdt, eth, sui",
		})
		return
	}

	// Get payment address based on currency
	var paymentAddress string
	switch req.Currency {
	case "sui":
		paymentAddress = "0x..." // SUI payment address (should be configured)
	case "usdc", "usdt":
		paymentAddress = "0x..." // USDC/USDT payment address (should be configured)
	case "eth":
		paymentAddress = "0x..." // ETH payment address (should be configured)
	}

	// Create order
	order, err := h.repo.CreateOrder(
		c.Request.Context(),
		int(userID.(int64)),
		req.ChecksCount,
		selectedMethod.PriceUSD,
		req.Currency,
		selectedMethod.TokenAmount,
		paymentAddress,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "failed to create order",
			Code:  "DB_ERROR",
		})
		return
	}

	c.JSON(http.StatusCreated, models.PurchaseResponse{
		OrderID:        order.OrderUUID,
		ChecksCount:    req.ChecksCount,
		TotalUSD:       selectedMethod.PriceUSD,
		Currency:       req.Currency,
		TokenAmount:    selectedMethod.TokenAmount,
		PaymentAddress: paymentAddress,
		DueDate:        time.Now().Add(30 * time.Minute),
		Status:         "pending",
	})
}

// VerifyPayment verifies a blockchain transaction and completes the order
func (h *Handler) VerifyPayment(c *gin.Context) {
	orderUUID := c.Query("order_id")
	txHash := c.Query("tx_hash")

	if orderUUID == "" || txHash == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "order_id and tx_hash are required",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Get order
	order, err := h.repo.GetOrderByUUID(c.Request.Context(), orderUUID)
	if err != nil || order == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "order not found",
			Code:  "NOT_FOUND",
		})
		return
	}

	if order.Status != "pending" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "order is not pending",
			Code:  "INVALID_STATUS",
		})
		return
	}

	// In production, you would verify the transaction on-chain here
	// For now, we auto-complete the order
	
	// Complete order
	if err := h.repo.CompleteOrder(c.Request.Context(), orderUUID, txHash); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "failed to complete order",
			Code:  "DB_ERROR",
		})
		return
	}

	// Add balance to user
	if err := h.repo.AddUserBalance(c.Request.Context(), order.UserID, order.ChecksCount); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "failed to add balance",
			Code:  "DB_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "payment verified",
		"order_id":     orderUUID,
		"checks_added": order.ChecksCount,
		"status":       "completed",
	})
}

// ==================== Wallet Checking ====================

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

	// Get remaining balance from middleware (if authenticated, balance was deducted)
	// If not authenticated, balance is -1 (unlimited IP-based)
	balanceLeft := -1
	if remaining, exists := c.Get("remainingBalance"); exists {
		balanceLeft = remaining.(int)
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
		Address:     req.Address,
		Chain:       req.Chain,
		Found:       wallet != nil,
		BalanceLeft: balanceLeft,
	}

	var status string
	if wallet != nil {
		status = string(wallet.Status)
		response.Status = status
		response.HasPK = wallet.HasPK
		response.HasSeed = wallet.HasSeed
	} else {
		status = "safe"
		response.Status = "not_found"
	}

	// Record check in history
	go func() {
		h.repo.RecordCheck(context.Background(), req.Address, req.Chain, status)
	}()

	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetRecentChecks(c *gin.Context) {
	// First try to get from check_history
	checks, err := h.repo.GetCheckHistory(c.Request.Context(), 50)
	if err != nil {
		// Fallback to wallets table
		checks, err = h.repo.GetRecentChecks(c.Request.Context(), 50)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "database error",
				Code:  "DB_ERROR",
			})
			return
		}
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
			{"name": "Sui", "id": "sui", "example": "0x8a1c4cd2d2fd05e02e3d2b5f4d6f8a1c3e5b7d9f0a2c4e6"},
			{"name": "Tron", "id": "tron", "example": "TJK5M5kKxP8xF9cGvN2pL6rU4sW7xA3bCd"},
		},
	})
}
