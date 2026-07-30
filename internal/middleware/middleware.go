package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/auth"
	"vauln-address/internal/config"
	"vauln-address/internal/models"
	"vauln-address/internal/repository"
)

type RateLimiter struct {
	repo  *repository.Repository
	cfg   *config.Config
}

func NewRateLimiter(repo *repository.Repository, cfg *config.Config) *RateLimiter {
	return &RateLimiter{
		repo: repo,
		cfg:  cfg,
	}
}

// CheckRequest for validation before rate limiting
type CheckRequest struct {
	Address string `json:"address"`
	Chain   string `json:"chain"`
}

func (rl *RateLimiter) Limit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		ctx := c.Request.Context()

		// Check if user is authenticated
		walletAddress, isAuthenticated := c.Get("userAddress")
		userChain, _ := c.Get("userChain")
		chainStr, _ := userChain.(string)

		// 1. Check IP rate limit
		rl.checkAndResetWindow(ctx, ip)

		rateLimit, err := rl.repo.GetRateLimit(ctx, ip)
		if err != nil {
			c.Next()
			return
		}

		if rateLimit == nil {
			err = rl.repo.ResetRateLimit(ctx, ip, time.Now())
			if err != nil {
				c.Next()
				return
			}
			rateLimit = &models.RateLimit{Count: 0, WindowStart: time.Now()}
		}

		// Check if authenticated user has purchased balance
		var purchasedBalance int
		hasPurchasedBalance := false
		if isAuthenticated && walletAddress != nil && chainStr != "" {
			purchasedBalance, err = rl.repo.GetUserBalance(ctx, walletAddress.(string), chainStr)
			if err == nil && purchasedBalance > 0 {
				hasPurchasedBalance = true
			}
		}

		// Calculate IP remaining
		ipRemaining := max(0, rl.cfg.FreeCheckLimit-rateLimit.Count)

		// Calculate total available: IP + purchased (only if has purchased)
		totalAvailable := ipRemaining
		if hasPurchasedBalance {
			totalAvailable += purchasedBalance
		}

		// Set rate limit headers
		c.Header("X-RateLimit-Limit", strconv.Itoa(totalAvailable))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(totalAvailable))
		c.Header("X-RateLimit-Used", strconv.Itoa(rateLimit.Count))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(nextMidnight().Unix(), 10))
		if hasPurchasedBalance {
			c.Header("X-RateLimit-Source", "mixed")
			c.Header("X-Balance-Available", strconv.Itoa(purchasedBalance))
			c.Header("X-IP-Remaining", strconv.Itoa(ipRemaining))
		} else {
			c.Header("X-RateLimit-Source", "ip")
		}

		// For /check endpoint, validate early to avoid wasting rate limit on bad requests
		if c.Request.URL.Path == "/api/check" && c.Request.Method == "POST" {
			// Read body for validation
			body, err := io.ReadAll(c.Request.Body)
			if err != nil {
				c.Next()
				return
			}
			// Restore body for handler
			c.Request.Body = io.NopCloser(strings.NewReader(string(body)))

			var req CheckRequest
			if err := json.Unmarshal(body, &req); err != nil {
				c.JSON(http.StatusBadRequest, models.ErrorResponse{
					Error: "invalid request body",
					Code:  "INVALID_REQUEST",
				})
				c.Abort()
				return
			}

			req.Address = strings.TrimSpace(req.Address)
			req.Chain = strings.ToLower(strings.TrimSpace(req.Chain))

			// Validate chain
			if !models.IsValidChain(req.Chain) {
				c.JSON(http.StatusBadRequest, models.ErrorResponse{
					Error:   "unsupported chain",
					Code:    "INVALID_CHAIN",
					Details: "supported chains: evm, btc, solana, sui, tron",
				})
				c.Abort()
				return
			}

			// Validate address format
			if !validateAddressFormat(req.Chain, req.Address) {
				c.JSON(http.StatusBadRequest, models.ErrorResponse{
					Error:   "invalid address format",
					Code:    "INVALID_ADDRESS",
					Details: fmt.Sprintf("invalid %s address format", req.Chain),
				})
				c.Abort()
				return
			}
		}

		// Check if total available is exhausted
		if totalAvailable <= 0 {
			windowEnd := nextMidnight()
			resetIn := time.Until(windowEnd)

			c.JSON(http.StatusTooManyRequests, models.ErrorResponse{
				Error:   "rate limit exceeded",
				Code:    "RATE_LIMIT_EXCEEDED",
				Details: formatRateLimitDetails(rateLimit.Count, rl.cfg.FreeCheckLimit, resetIn),
			})
			c.Abort()
			return
		}

		// Use purchased balance first if available
		if hasPurchasedBalance {
			c.Set("pendingBalanceDeduction", true)
			c.Set("pendingDeductionAddress", walletAddress.(string))
			c.Set("pendingDeductionChain", chainStr)
			c.Set("pendingDeductionBalance", purchasedBalance)
			c.Set("usingBalance", true)
			c.Next()
			return
		}

		// No purchased balance - use IP-based check
		if err := rl.repo.IncrementRateLimit(ctx, ip, rateLimit.WindowStart); err != nil {
			c.Next()
			return
		}

		c.Next()
	}
}

// nextMidnight returns the next midnight (00:00) UTC
func nextMidnight() time.Time {
	now := time.Now().UTC()
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	return tomorrow
}

// setRateLimitHeaders sets common rate limit headers
func (rl *RateLimiter) setRateLimitHeaders(c *gin.Context, rateLimit *models.RateLimit, isUserAuthenticated bool) {
	limit := rl.cfg.FreeCheckLimit
	used := rateLimit.Count
	remaining := max(0, limit-used)
	windowEnd := nextMidnight()

	c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
	c.Header("X-RateLimit-Used", strconv.Itoa(used))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(windowEnd.Unix(), 10))

	if isUserAuthenticated {
		c.Header("X-RateLimit-Source", "balance")
	} else {
		c.Header("X-RateLimit-Source", "ip")
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (rl *RateLimiter) checkAndResetWindow(ctx context.Context, ip string) {
	rateLimit, err := rl.repo.GetRateLimit(ctx, ip)
	if err != nil || rateLimit == nil {
		return
	}

	// Check if we're past midnight and need to reset
	midnight := nextMidnight()
	if time.Now().UTC().After(midnight) {
		_ = rl.repo.ResetRateLimit(ctx, ip, time.Now())
	}
}

func formatRateLimitDetails(used, limit int, resetIn time.Duration) string {
	return fmt.Sprintf("checks exhausted. Reset in %s", formatDuration(resetIn))
}

func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%d minutes", minutes)
}

// Address validation regexes
var (
	evmRegex   = regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`)
	btcRegex   = regexp.MustCompile(`^(bc1|[13])[a-zA-HJ-NP-Z0-9]{25,39}$`)
	solRegex   = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`)
	suiRegex   = regexp.MustCompile(`^0x[a-fA-F0-9]{64}$`)
	tronRegex  = regexp.MustCompile(`^T[A-HJ-NP-Za-km-z1-9]{33}$`)
)

func validateAddressFormat(chain, address string) bool {
	if address == "" {
		return false
	}
	
	switch chain {
	case "evm":
		return evmRegex.MatchString(address)
	case "btc":
		return btcRegex.MatchString(address)
	case "solana":
		return solRegex.MatchString(address)
	case "sui":
		return suiRegex.MatchString(address)
	case "tron":
		return tronRegex.MatchString(address)
	default:
		return false
	}
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// AuthMiddleware validates JWT tokens for protected routes
func AuthMiddleware(authService *auth.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.Next()
			return
		}

		tokenString := parts[1]
		claims, err := authService.ValidateToken(tokenString)
		if err != nil {
			c.Next()
			return
		}

		// Set user info in context
		c.Set("userAddress", claims.Address)
		c.Set("userChain", claims.Chain)
		c.Next()
	}
}

// RequireAuth ensures the user is authenticated
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get("userAddress"); !exists {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error: "authentication required",
				Code:  "UNAUTHORIZED",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// AdminMiddleware validates admin API key
func AdminMiddleware(adminAPIKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		adminKey := c.GetHeader("X-Admin-Key")
		if adminKey == "" {
			adminKey = c.Query("admin_key")
		}
		
		if adminKey == "" || adminKey != adminAPIKey {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error: "admin access required",
				Code:  "ADMIN_REQUIRED",
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}
