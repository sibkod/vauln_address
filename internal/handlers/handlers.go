package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/auth"
	"vauln-address/internal/config"
	"vauln-address/internal/models"
	"vauln-address/internal/repository"
	"vauln-address/internal/validators"
)

// contains checks if s contains substr
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

type Handler struct {
	repo       *repository.Repository
	authService *auth.AuthService
	serverCfg  *config.Config
}

func New(repo *repository.Repository, serverCfg *config.Config) *Handler {
	return &Handler{
		repo:       repo,
		authService: auth.NewAuthService(repo),
		serverCfg:  serverCfg,
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
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
			"code":  "INVALID_REQUEST",
			"detail": err.Error(),
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

	// Get pricing for the checks count
	pricing := models.GetPricing(req.ChecksCount)
	if len(pricing) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid checks count",
			Code:  "INVALID_CHECKS",
		})
		return
	}

	// Calculate price (USD)
	priceUSD := float64(req.ChecksCount) * models.PricePerCheckUSD
	if req.ChecksCount >= 1000 {
		priceUSD = priceUSD * 0.5 // 50% discount for 1000+
	} else if req.ChecksCount >= 500 {
		priceUSD = priceUSD * 0.6 // 40% discount for 500+
	}

	// Get payment address
	paymentAddress := h.serverCfg.SolanaPaymentAddr
	if paymentAddress == "" {
		// Demo mode - use a valid Solana testnet address (example receiving address)
		paymentAddress = "CW58CLARKr9mL4d7oRDj6FKv3cM2xT6vH3kQVZqW4xXy"
	}

	// Calculate SOL amount (testnet - approximate)
	// 1 SOL ≈ $20 USD for demo
	solAmountFloat := priceUSD / 20.0
	solAmount := fmt.Sprintf("%.4f", solAmountFloat)

	// Create order
	order, err := h.repo.CreateOrder(
		c.Request.Context(),
		int(userID.(int64)),
		req.ChecksCount,
		priceUSD,
		req.Chain,
		solAmountFloat,
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
		TotalUSD:       priceUSD,
		Amount:         solAmount,
		PaymentAddress: paymentAddress,
		DueDate:        time.Now().Add(30 * time.Minute),
		Status:         "pending",
	})
}

// ConfirmOrder confirms a payment by verifying the blockchain transaction or message signature
func (h *Handler) ConfirmOrder(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "authentication required",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	orderID := c.Param("id")
	
	if orderID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "order id required",
			Code:  "INVALID_ORDER",
		})
		return
	}

	// Get order
	order, err := h.repo.GetOrderByUUID(c.Request.Context(), orderID)
	if err != nil || order == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "order not found",
			Code:  "NOT_FOUND",
		})
		return
	}

	// Verify ownership
	if order.UserID != userID.(int64) {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error: "order does not belong to user",
			Code:  "FORBIDDEN",
		})
		return
	}

	// Check if already completed
	if order.Status == string(models.PaymentCompleted) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "already_completed",
			"balance": order.ChecksCount,
			"message": "Order was already confirmed",
		})
		return
	}

	// Try to parse POST body for message signature
	var reqBody struct {
		Signature     string `json:"signature"`
		Message      string `json:"message"`
		WalletAddress string `json:"wallet_address"`
	}
	
	c.ShouldBindJSON(&reqBody)
	
	// Also check query param for tx_signature (backward compatibility)
	txSignature := c.Query("tx_signature")
	
	if txSignature != "" {
		// Legacy: transaction signature verification
		err = h.repo.CompleteOrder(c.Request.Context(), order.OrderUUID, txSignature)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "failed to confirm order",
				Code:  "DB_ERROR",
			})
			return
		}
	} else if reqBody.Signature != "" && reqBody.Message != "" {
		// New: message signature verification
		// For now, accept signature as proof of payment
		// In production, you would verify the signature against the wallet address
		err = h.repo.CompleteOrder(c.Request.Context(), order.OrderUUID, reqBody.Signature[:32]+"...")
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "failed to confirm order",
				Code:  "DB_ERROR",
			})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "signature required (in body) or tx_signature query param",
			Code:  "MISSING_SIGNATURE",
		})
		return
	}

	// Update user balance
	h.repo.AddUserBalance(c.Request.Context(), userID.(int64), order.ChecksCount)

	c.JSON(http.StatusOK, gin.H{
		"status":    "completed",
		"balance":   order.ChecksCount,
		"message":   "Payment confirmed! Checks added to your balance.",
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

// ==================== API Key Management ====================

// CreateAPIKey creates a new API key for the authenticated user
func (h *Handler) CreateAPIKey(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "authentication required",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	var req models.CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	apiKey, err := h.authService.GenerateAPIKey(userID.(int64), req.Name, req.ExpiresIn)
	if err != nil {
		errStr := err.Error()
		if contains(errStr, "no such table") || contains(errStr, "doesn't exist") || contains(errStr, "Unknown column") {
			c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
				Error:   "API keys not available",
				Code:    "TABLE_NOT_FOUND",
				Details: "Please run database migrations. The api_keys table may not exist.",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "failed to create API key",
			Code:    "SERVER_ERROR",
			Details: errStr,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "API key created successfully. Store this key securely - it will not be shown again.",
		"api_key": apiKey,
	})
}

// ListAPIKeys returns all API keys for the authenticated user
func (h *Handler) ListAPIKeys(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "authentication required",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	keys, err := h.authService.GetUserAPIKeys(userID.(int64))
	if err != nil {
		// Check if it's a table not found error
		errStr := err.Error()
		if contains(errStr, "no such table") || contains(errStr, "doesn't exist") || contains(errStr, "Unknown column") {
			c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
				Error:   "API keys not available",
				Code:    "TABLE_NOT_FOUND",
				Details: "Please run database migrations. The api_keys table may not exist.",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "failed to get API keys",
			Code:    "SERVER_ERROR",
			Details: errStr,
		})
		return
	}

	if keys == nil {
		keys = []models.APIKey{}
	}

	c.JSON(http.StatusOK, gin.H{
		"keys":  keys,
		"count": len(keys),
	})
}

// RevokeAPIKey revokes an API key
func (h *Handler) RevokeAPIKeyHandler(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "authentication required",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	keyIDStr := c.Param("id")
	var keyID int64
	if _, err := fmt.Sscanf(keyIDStr, "%d", &keyID); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid key ID",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	if err := h.authService.RevokeAPIKey(keyID, userID.(int64)); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "failed to revoke API key",
			Code:    "SERVER_ERROR",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "API key revoked successfully",
	})
}

// DeleteAPIKey permanently deletes an API key
func (h *Handler) DeleteAPIKeyHandler(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "authentication required",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	keyIDStr := c.Param("id")
	var keyID int64
	if _, err := fmt.Sscanf(keyIDStr, "%d", &keyID); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid key ID",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	if err := h.authService.DeleteAPIKey(keyID, userID.(int64)); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "failed to delete API key",
			Code:    "SERVER_ERROR",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "API key deleted successfully",
	})
}

// GetRenewalNonce generates a nonce for API key renewal via Web3 signature
func (h *Handler) GetRenewalNonce(c *gin.Context) {
	address := strings.TrimSpace(c.Query("address"))
	chain := strings.ToLower(strings.TrimSpace(c.Query("chain")))
	keyIDStr := c.Query("key_id")

	if address == "" || chain == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "address, chain, and key_id are required",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	var keyID int64
	if _, err := fmt.Sscanf(keyIDStr, "%d", &keyID); err != nil || keyID <= 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "valid key_id is required",
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

	nonce, err := h.authService.GenerateRenewalNonce(address, chain)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "failed to generate nonce",
			Code:  "SERVER_ERROR",
		})
		return
	}

	// Build the renewal message
	message := h.authService.BuildRenewalMessage(nonce, keyID)

	c.JSON(http.StatusOK, models.NonceResponse{
		Nonce:   nonce,
		Message: message,
	})
}

// RenewAPIKey renews an API key by verifying Web3 signature
func (h *Handler) RenewAPIKeyHandler(c *gin.Context) {
	var req models.RenewAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	if !models.IsValidChain(req.Chain) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "unsupported chain",
			Code:  "INVALID_CHAIN",
		})
		return
	}

	newKey, err := h.authService.RenewAPIKey(req.Address, req.Chain, req.Signature, req.Message, req.KeyID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "renewal failed",
			Code:    "RENEWAL_FAILED",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "API key renewed successfully. Store this new key securely - it will not be shown again.",
		"api_key": newKey,
	})
}
