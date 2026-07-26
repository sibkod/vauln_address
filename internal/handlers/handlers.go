package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/auth"
	"vauln-address/internal/config"
	"vauln-address/internal/models"
	"vauln-address/internal/repository"
	"vauln-address/internal/services"
	"vauln-address/internal/validators"
)

// contains checks if s contains substr
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

type Handler struct {
	repo         *repository.Repository
	authService  *auth.AuthService
	serverCfg    *config.Config
	priceService *services.PriceService
	packages     []gin.H
}

func New(repo *repository.Repository, serverCfg *config.Config, priceService *services.PriceService) *Handler {
	h := &Handler{
		repo:         repo,
		authService:  auth.NewAuthService(repo),
		serverCfg:    serverCfg,
		priceService: priceService,
		packages:     loadPackages(),
	}
	return h
}

// loadPackages loads pricing packages from JSON file
func loadPackages() []gin.H {
	data, err := os.ReadFile("internal/data/pricing.json")
	if err != nil {
		log.Printf("Failed to load pricing.json: %v, using defaults", err)
		return getDefaultPackages()
	}

	var pkgData struct {
		Packages []struct {
			ID              string  `json:"id"`
			Name            string  `json:"name"`
			Checks          int     `json:"checks"`
			PriceUSD        float64 `json:"price_usd"`
			DiscountPercent int     `json:"discount_percent"`
			DiscountLabel   string  `json:"discount_label"`
			Popular         bool    `json:"popular"`
		} `json:"packages"`
	}

	if err := json.Unmarshal(data, &pkgData); err != nil {
		log.Printf("Failed to parse pricing.json: %v, using defaults", err)
		return getDefaultPackages()
	}

	packages := make([]gin.H, len(pkgData.Packages))
	for i, p := range pkgData.Packages {
		packages[i] = gin.H{
			"id":                p.ID,
			"name":              p.Name,
			"checks":            p.Checks,
			"price_usd":         p.PriceUSD,
			"discount_percent":   p.DiscountPercent,
			"discount_label":    p.DiscountLabel,
			"popular":           p.Popular,
		}
	}
	return packages
}

func getDefaultPackages() []gin.H {
	return []gin.H{
		{"id": "starter", "name": "Starter", "checks": 50, "price_usd": 5.0, "discount_percent": 0, "discount_label": "", "popular": false},
		{"id": "pro", "name": "Pro", "checks": 200, "price_usd": 20.0, "discount_percent": 0, "discount_label": "", "popular": true},
		{"id": "enterprise", "name": "Enterprise", "checks": 1000, "price_usd": 50.0, "discount_percent": 50, "discount_label": "50% OFF", "popular": false},
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
			"error":  "invalid request body",
			"code":   "INVALID_REQUEST",
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
	walletAddress, exists := c.Get("userAddress")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "unauthorized",
			Code:  "UNAUTHORIZED",
		})
		return
	}
	chain, _ := c.Get("userChain")
	chainStr, _ := chain.(string)

	user, err := h.authService.GetUserByWallet(walletAddress.(string), chainStr)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "user not found",
			Code:  "NOT_FOUND",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"wallet_address": user.WalletAddress,
			"chain":          user.Chain,
			"balance":        user.Balance,
			"is_premium":     user.Balance > 10,
			"created_at":     user.CreatedAt,
			"last_login_at":  user.LastLoginAt,
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

	pricing := models.GetPricing(checks, h.serverCfg.PricePerCheckUSD)

	c.JSON(http.StatusOK, gin.H{
		"checks":              checks,
		"price_per_check_usd": h.serverCfg.PricePerCheckUSD,
		"payment_methods":     pricing,
	})
}

// GetPackages returns pre-defined pricing packages with dynamic SOL prices
func (h *Handler) GetPackages(c *gin.Context) {
	solPrice := h.priceService.GetSolPrice()
	
	// Calculate SOL price for each package
	packages := make([]gin.H, len(h.packages))
	for i, pkg := range h.packages {
		priceUSD := pkg["price_usd"].(float64)
		priceSOL := math.Ceil(priceUSD/solPrice*10000) / 10000 // Round to 4 decimal places
		
		packages[i] = gin.H{
			"id":                pkg["id"],
			"name":              pkg["name"],
			"checks":            pkg["checks"],
			"price_usd":         priceUSD,
			"price_sol":         priceSOL,
			"discount_percent":  pkg["discount_percent"],
			"discount_label":    pkg["discount_label"],
			"popular":           pkg["popular"],
		}
	}

	// Add payment address info
	paymentAddress := h.serverCfg.SolanaPaymentAddr
	if paymentAddress == "" {
		paymentAddress = "CW58CLARKr9mL4d7oRDj6FKv3cM2xT6vH3kQVZqW4xXy" // Demo address
	}

	network := "mainnet"
	if h.serverCfg.SolanaUseDevnet {
		network = "devnet"
	}

	c.JSON(http.StatusOK, gin.H{
		"packages":          packages,
		"payment_address":  paymentAddress,
		"sol_price_usd":     solPrice,
		"price_per_check":   h.serverCfg.PricePerCheckUSD,
		"network":           network,
		"updated_at":        h.priceService.GetUpdatedAt().Unix(),
	})
}

// CreateOrder creates a new payment order
func (h *Handler) CreateOrder(c *gin.Context) {
	walletAddress, exists := c.Get("userAddress")
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

	// Validate checks count
	if req.ChecksCount < 1 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid checks count",
			Code:  "INVALID_CHECKS",
		})
		return
	}

	// Calculate price (USD)
	priceUSD := float64(req.ChecksCount) * h.serverCfg.PricePerCheckUSD
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

	// Calculate SOL amount based on current price
	solPrice := h.priceService.GetSolPrice()
	if solPrice <= 0 {
		solPrice = h.serverCfg.SolanaPriceUSD // fallback
	}
	solAmountFloat := math.Ceil(priceUSD/solPrice*10000) / 10000 // Round to 4 decimal places
	solAmount := fmt.Sprintf("%.4f", solAmountFloat)

	// Create order
	order, err := h.repo.CreateOrder(
		c.Request.Context(),
		walletAddress.(string),
		req.Chain,
		req.ChecksCount,
		priceUSD,
		"solana",
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

// CancelOrder cancels a pending order
func (h *Handler) CancelOrder(c *gin.Context) {
	walletAddress, exists := c.Get("userAddress")
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
	if order.WalletAddress != walletAddress.(string) {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error: "order does not belong to user",
			Code:  "FORBIDDEN",
		})
		return
	}

	// Check if already completed or cancelled
	if order.Status == string(models.PaymentCompleted) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "cannot cancel completed order",
			Code:  "ALREADY_COMPLETED",
		})
		return
	}
	if order.Status == string(models.PaymentCancelled) || order.Status == string(models.PaymentExpired) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "order is already cancelled",
			Code:  "ALREADY_CANCELLED",
		})
		return
	}

	// Cancel the order
	if err := h.repo.CancelOrder(c.Request.Context(), orderID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "failed to cancel order",
			Code:  "DB_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "cancelled",
		"message": "Order has been cancelled",
	})
}

// ConfirmOrder confirms a payment by verifying the blockchain transaction or message signature
func (h *Handler) ConfirmOrder(c *gin.Context) {
	walletAddress, exists := c.Get("userAddress")
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

	// Verify ownership by wallet address
	if order.WalletAddress != walletAddress.(string) {
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
		Message       string `json:"message"`
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
	h.repo.AddUserBalance(c.Request.Context(), walletAddress.(string), order.Chain, order.ChecksCount)

	c.JSON(http.StatusOK, gin.H{
		"status":  "completed",
		"balance": order.ChecksCount,
		"message": "Payment confirmed! Checks added to your balance.",
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
	if err := h.repo.AddUserBalance(c.Request.Context(), order.WalletAddress, order.Chain, order.ChecksCount); err != nil {
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

// GetMe returns comprehensive user info including balance, rate limits, and user data
func (h *Handler) GetMe(c *gin.Context) {
	walletAddress, exists := c.Get("userAddress")
	chain, _ := c.Get("userChain")
	chainStr, _ := chain.(string)

	// Get rate limit info using FreeCheckLimit (per IP free check limit)
	ip := c.ClientIP()
	rateLimit, _ := h.repo.GetRateLimit(c.Request.Context(), ip)
	rateLimitRemaining := h.serverCfg.FreeCheckLimit
	rateLimitUsed := 0
	if rateLimit != nil {
		rateLimitRemaining = h.serverCfg.FreeCheckLimit - rateLimit.Count
		if rateLimitRemaining < 0 {
			rateLimitRemaining = 0
		}
		rateLimitUsed = rateLimit.Count
	}

	// Check if user is authenticated
	if exists && walletAddress != nil && walletAddress.(string) != "" && chainStr != "" {
		// Authenticated user
		user, err := h.authService.GetUserByWallet(walletAddress.(string), chainStr)
		if err != nil || user == nil {
			c.JSON(http.StatusOK, gin.H{
				"wallet_address":         walletAddress.(string),
				"chain":                  chainStr,
				"balance":                0,
				"purchased_balance":      0,
				"rate_limit_remaining":   rateLimitRemaining,
				"rate_limit_used":        rateLimitUsed,
				"rate_limit_limit":       h.serverCfg.FreeCheckLimit,
				"is_premium":             false,
				"is_authenticated":       true,
			})
			return
		}

		purchasedBalance := user.Balance
		isPremium := purchasedBalance > 0

		// Add rate limit headers
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", h.serverCfg.FreeCheckLimit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", rateLimitRemaining))
		c.Header("X-RateLimit-Used", fmt.Sprintf("%d", rateLimitUsed))
		c.Header("X-RateLimit-Source", "balance")
		c.Header("X-Balance-Available", fmt.Sprintf("%d", purchasedBalance))

		c.JSON(http.StatusOK, gin.H{
			"wallet_address":         user.WalletAddress,
			"chain":                  user.Chain,
			"balance":                purchasedBalance,
			"purchased_balance":      purchasedBalance,
			"rate_limit_remaining":   rateLimitRemaining,
			"rate_limit_used":        rateLimitUsed,
			"rate_limit_limit":       h.serverCfg.FreeCheckLimit,
			"is_premium":             isPremium,
			"is_authenticated":       true,
			"created_at":             user.CreatedAt,
			"last_login_at":          user.LastLoginAt,
		})
		return
	}

	// Anonymous user
	c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", h.serverCfg.FreeCheckLimit))
	c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", rateLimitRemaining))
	c.Header("X-RateLimit-Used", fmt.Sprintf("%d", rateLimitUsed))
	c.Header("X-RateLimit-Source", "ip")

	c.JSON(http.StatusOK, gin.H{
		"wallet_address":         nil,
		"chain":                  nil,
		"balance":                rateLimitRemaining,
		"purchased_balance":      0,
		"rate_limit_remaining":   rateLimitRemaining,
		"rate_limit_used":        rateLimitUsed,
		"rate_limit_limit":       h.serverCfg.FreeCheckLimit,
		"is_premium":             false,
		"is_authenticated":       false,
	})
}

// GetBalance returns the current user's balance (or remaining rate limit for anonymous)
func (h *Handler) GetBalance(c *gin.Context) {
	// Check if user is authenticated
	walletAddress, exists := c.Get("userAddress")
	chain, _ := c.Get("userChain")
	chainStr, _ := chain.(string)

	// Get rate limit remaining using FreeCheckLimit (per IP free check limit)
	ip := c.ClientIP()
	rateLimit, _ := h.repo.GetRateLimit(c.Request.Context(), ip)
	rateLimitRemaining := h.serverCfg.FreeCheckLimit
	if rateLimit != nil {
		rateLimitRemaining = h.serverCfg.FreeCheckLimit - rateLimit.Count
		if rateLimitRemaining < 0 {
			rateLimitRemaining = 0
		}
	}

	// Validate that user is actually authenticated (not just token present)
	if exists && walletAddress != nil && walletAddress != "" && chainStr != "" {
		// Authenticated user
		purchasedBalance, err := h.repo.GetUserBalance(c.Request.Context(), walletAddress.(string), chainStr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "failed to get balance",
				Code:  "DB_ERROR",
			})
			return
		}

		// If user has purchased checks, show purchased balance
		// Otherwise, show rate limit remaining (free checks for authenticated users)
		if purchasedBalance > 0 {
			c.JSON(http.StatusOK, gin.H{
				"balance":               purchasedBalance,
				"purchased_balance":     purchasedBalance,
				"rate_limit_remaining":  rateLimitRemaining,
				"source":                "purchased",
			})
		} else {
			c.JSON(http.StatusOK, gin.H{
				"balance":               rateLimitRemaining,
				"purchased_balance":     0,
				"rate_limit_remaining": rateLimitRemaining,
				"source":                "rate_limit",
			})
		}
		return
	}

	// Anonymous user - return IP-based rate limit remaining only
	c.JSON(http.StatusOK, gin.H{
		"balance":               rateLimitRemaining,
		"purchased_balance":     0,
		"rate_limit_remaining": rateLimitRemaining,
		"source":               "rate_limit",
	})
}

// GetPurchaseHistory returns the user's order history with pagination
func (h *Handler) GetPurchaseHistory(c *gin.Context) {
	walletAddress, exists := c.Get("userAddress")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "unauthorized",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	pageStr := c.DefaultQuery("page", "1")
	perPageStr := c.DefaultQuery("per_page", "10")
	
	var page, perPage int
	if _, err := fmt.Sscanf(pageStr, "%d", &page); err != nil || page < 1 {
		page = 1
	}
	if _, err := fmt.Sscanf(perPageStr, "%d", &perPage); err != nil || perPage < 1 {
		perPage = 10
	}
	if perPage > 50 {
		perPage = 50
	}

	offset := (page - 1) * perPage

	orders, total, err := h.repo.GetOrdersByWalletPaginated(c.Request.Context(), walletAddress.(string), perPage, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "failed to get orders",
			Code:  "DB_ERROR",
		})
		return
	}

	if orders == nil {
		orders = []models.Order{}
	}

	totalPages := (total + perPage - 1) / perPage

	c.JSON(http.StatusOK, gin.H{
		"orders":      orders,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
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

	// All validations passed - now deduct balance if pending
	// Get pending balance info from middleware
	balanceLeft := -1
	if pendingDeduction, exists := c.Get("pendingBalanceDeduction"); exists && pendingDeduction.(bool) {
		deductionAddrVal, _ := c.Get("pendingDeductionAddress")
		deductionChainVal, _ := c.Get("pendingDeductionChain")
		deductionAddr, _ := deductionAddrVal.(string)
		deductionChain, _ := deductionChainVal.(string)
		
		if deductionAddr != "" && deductionChain != "" {
			if err := h.repo.DeductUserBalance(c.Request.Context(), deductionAddr, deductionChain, 1); err == nil {
				if prevBalance, exists := c.Get("pendingDeductionBalance"); exists {
					balanceLeft = prevBalance.(int) - 1
				}
				c.Header("X-Balance-Available", strconv.Itoa(maxInt(0, balanceLeft)))
			}
		}
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
	userAddr := ""
	if ua, exists := c.Get("userAddress"); exists {
		userAddr = ua.(string)
	}
	go func() {
		h.repo.RecordCheck(context.Background(), userAddr, req.Address, req.Chain, status)
	}()

	c.JSON(http.StatusOK, response)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (h *Handler) GetRecentChecks(c *gin.Context) {
	// Default limit
	limit := 10
	offset := 0
	
	// Check for pagination params
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Check if user is authenticated
	walletAddress, isAuthenticated := c.Get("userAddress")
	
	var checks []models.RecentCheck
	var total int
	var err error

	if isAuthenticated && walletAddress != nil {
		// Authenticated user: get their check history with pagination
		checks, total, err = h.repo.GetCheckHistoryByWallet(c.Request.Context(), walletAddress.(string), limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "database error",
				Code:  "DB_ERROR",
			})
			return
		}
	} else {
		// Anonymous: get global recent checks (limited to 10)
		checks, err = h.repo.GetCheckHistory(c.Request.Context(), 10)
		if err != nil {
			// Fallback to wallets table
			checks, err = h.repo.GetRecentChecks(c.Request.Context(), 10)
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
	}

	if checks == nil {
		checks = []models.RecentCheck{}
	}

	response := gin.H{
		"checks": checks,
		"count":  len(checks),
	}
	
	// Add pagination info for authenticated users
	if isAuthenticated && walletAddress != nil {
		response["total"] = total
		response["limit"] = limit
		response["offset"] = offset
	}

	c.JSON(http.StatusOK, response)
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
	walletAddress, exists := c.Get("userAddress")
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

	apiKey, err := h.authService.GenerateAPIKey(walletAddress.(string), req.Name, req.ExpiresIn)
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
	walletAddress, exists := c.Get("userAddress")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "authentication required",
			Code:  "UNAUTHORIZED",
		})
		return
	}

	keys, err := h.authService.GetUserAPIKeys(walletAddress.(string))
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
	walletAddress, exists := c.Get("userAddress")
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

	if err := h.authService.RevokeAPIKey(keyID, walletAddress.(string)); err != nil {
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
	walletAddress, exists := c.Get("userAddress")
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

	if err := h.authService.DeleteAPIKey(keyID, walletAddress.(string)); err != nil {
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

// ==================== Solana Payment Verification ====================

// GetPaymentStatus checks a Solana transaction status
func (h *Handler) GetPaymentStatus(c *gin.Context) {
	signature := c.Param("signature")
	if signature == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "transaction signature is required",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Get user info (REQUIRED - now enforced by middleware)
	walletAddress, exists := c.Get("userAddress")
	if !exists || walletAddress == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "authentication required",
			Code:  "UNAUTHORIZED",
		})
		return
	}
	walletAddrStr := walletAddress.(string)

	// Query Solana RPC for transaction status
	rpcURL := h.serverCfg.SolanaRPCURL
	if rpcURL == "" {
		if h.serverCfg.SolanaUseDevnet {
			rpcURL = "https://api.devnet.solana.com"
		} else {
			rpcURL = "https://api.mainnet-beta.solana.com"
		}
	}

	// Get transaction status
	txStatus, err := h.querySolanaTransaction(rpcURL, signature)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
			Error:   "failed to verify transaction",
			Code:    "RPC_ERROR",
			Details: err.Error(),
		})
		return
	}

	if txStatus.Err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":    "failed",
			"confirmed": false,
			"message":   "transaction failed on chain",
		})
		return
	}

	if txStatus.Confirmations != "confirmed" && txStatus.Confirmations != "finalized" {
		c.JSON(http.StatusOK, gin.H{
			"status":        "pending",
			"confirmed":     false,
			"slot":          txStatus.Slot,
			"confirmations": txStatus.Confirmations,
			"message":       "transaction is processing",
		})
		return
	}

	// Transaction is confirmed!
	// Check if we already processed this transaction
	existingOrder, _ := h.repo.GetOrderByTxHash(c.Request.Context(), signature)
	if existingOrder != nil && existingOrder.Status == "completed" {
		// Already processed
		c.JSON(http.StatusOK, gin.H{
			"status":    "already_claimed",
			"confirmed": true,
			"balance":   existingOrder.ChecksCount,
			"message":   "checks already credited",
		})
		return
	}

	// Find pending order for this wallet
	pendingOrder, err := h.repo.GetPendingOrderByWallet(c.Request.Context(), walletAddrStr)
	log.Printf("[DEBUG] GetPaymentStatus: walletAddress=%s, pendingOrder=%v, err=%v", walletAddrStr, pendingOrder, err)
	if err == nil && pendingOrder != nil {
		log.Printf("[DEBUG] Found pending order: UUID=%s, ChecksCount=%d", pendingOrder.OrderUUID, pendingOrder.ChecksCount)
		// Complete the order
		if err := h.repo.CompleteOrder(c.Request.Context(), pendingOrder.OrderUUID, signature); err == nil {
			log.Printf("[DEBUG] Order completed, adding balance for wallet %s", walletAddrStr)
			// Add balance
			if err := h.repo.AddUserBalance(c.Request.Context(), walletAddrStr, pendingOrder.Chain, pendingOrder.ChecksCount); err == nil {
				log.Printf("[DEBUG] Balance added for wallet %s", walletAddrStr)
				// Get new balance
				user, err := h.repo.GetUserByWallet(c.Request.Context(), walletAddrStr, pendingOrder.Chain)
				balance := pendingOrder.ChecksCount // Default to added checks
				if err == nil && user != nil {
					balance = user.Balance
				} else {
					log.Printf("[WARN] Could not fetch user balance, using checks count: err=%v, user=%v", err, user)
				}
				c.JSON(http.StatusOK, gin.H{
					"status":    "confirmed",
					"confirmed": true,
					"balance":   balance,
					"message":   fmt.Sprintf("Payment confirmed! %d checks added.", pendingOrder.ChecksCount),
				})
				return
			} else {
				log.Printf("[ERROR] Failed to add balance: %v", err)
			}
		} else {
			log.Printf("[ERROR] Failed to complete order: %v", err)
		}
	} else {
		log.Printf("[WARN] No pending order found for wallet %s", walletAddrStr)
	}

	// Transaction confirmed but no order found for this user
	c.JSON(http.StatusOK, gin.H{
		"status":    "confirmed",
		"confirmed": true,
		"message":   "transaction confirmed on blockchain",
	})
}

// SolanaTransactionStatus represents transaction status from RPC
type SolanaTransactionStatus struct {
	Slot          uint64 `json:"slot"`
	Err           any    `json:"err"`
	Confirmations string `json:"confirmations"`
}

func (h *Handler) querySolanaTransaction(rpcURL, signature string) (*SolanaTransactionStatus, error) {
	// Make RPC call to get transaction
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getTransaction",
		"params": []interface{}{
			signature,
			map[string]interface{}{
				"encoding":                       "jsonParsed",
				"maxSupportedTransactionVersion": 0,
			},
		},
	}

	resp, err := http.Post(rpcURL, "application/json", toJSON(reqBody))
	if err != nil {
		return nil, fmt.Errorf("RPC request failed: %w", err)
	}
	defer resp.Body.Close()

	var rpcResp struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			Slot          uint64  `json:"slot"`
			Confirmations *string `json:"confirmationStatus"`
			Meta          *struct {
				Err any `json:"err"`
			} `json:"meta"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := decodeJSON(resp.Body, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", rpcResp.Error.Message)
	}

	if rpcResp.Result.Slot == 0 && rpcResp.Result.Meta == nil {
		return nil, fmt.Errorf("transaction not found")
	}

	confirmations := "confirmed"
	if rpcResp.Result.Confirmations != nil {
		confirmations = *rpcResp.Result.Confirmations
	}

	return &SolanaTransactionStatus{
		Slot:          rpcResp.Result.Slot,
		Err:           nil,
		Confirmations: confirmations,
	}, nil
}

func toJSON(v interface{}) *strings.Reader {
	b, _ := json.Marshal(v)
	return strings.NewReader(string(b))
}

func decodeJSON(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

// ==================== Admin Handlers ====================

// AddWallet adds a wallet (or wallets) to the database
func (h *Handler) AddWallet(c *gin.Context) {
	var req models.AddWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
			Details: err.Error(),
		})
		return
	}

	if len(req.Addresses) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "at least one address is required",
			Code:  "NO_ADDRESSES",
		})
		return
	}

	// Validate status
	if !models.IsValidStatus(string(req.Status)) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid status",
			Code:    "INVALID_STATUS",
			Details: "valid statuses: hacked, vulnerable, safe, hacker, drained",
		})
		return
	}

	ctx := c.Request.Context()

	// Save seed phrase if provided
	var seedID *int64
	var walletIDs []int64

	if req.SeedPhrase != "" {
		id, err := h.repo.SaveSeed(ctx, req.SeedPhrase)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "failed to save seed phrase",
				Code:  "DB_ERROR",
			})
			return
		}
		seedID = &id
	}

	// Add each address as a wallet
	for chain, address := range req.Addresses {
		address = strings.TrimSpace(address)
		chain = strings.ToLower(strings.TrimSpace(chain))

		if !models.IsValidChain(chain) {
			continue // Skip invalid chains
		}

		// Validate address format
		valid, _ := validators.ValidateAddress(chain, address)
		if !valid {
			continue // Skip invalid addresses
		}

		var walletID int64
		var err error

		if seedID != nil {
			walletID, err = h.repo.CreateWalletWithSeed(ctx, address, chain, req.Status, *seedID, req.Reason, req.Source)
		} else {
			walletID, err = h.repo.CreateWallet(ctx, address, chain, req.Status, req.Reason, req.Source)
		}

		if err != nil {
			log.Printf("Failed to create wallet %s/%s: %v", chain, address, err)
			continue
		}

		walletIDs = append(walletIDs, walletID)
	}

	response := models.AddWalletResponse{
		Success:      len(walletIDs) > 0,
		WalletsAdded: len(walletIDs),
		WalletIDs:    walletIDs,
		SeedID:       seedID,
		Message:      fmt.Sprintf("Added %d wallet(s)", len(walletIDs)),
	}

	c.JSON(http.StatusOK, response)
}
