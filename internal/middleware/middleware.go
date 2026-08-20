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

		// Add rate limit headers for all responses
		rl.setRateLimitHeaders(c, rateLimit, isAuthenticated && walletAddress != nil && chainStr != "")

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

					// Check IP limit using FreeCheckLimit
		if rateLimit.Count >= rl.cfg.FreeCheckLimit {
			// IP limit exhausted
			// For authenticated users: try to use their balance as fallback
			if isAuthenticated && walletAddress != nil && chainStr != "" {
				balance, err := rl.repo.GetUserBalance(ctx, walletAddress.(string), chainStr)
				if err == nil && balance > 0 {
					// Set flag for handler to deduct balance after successful check
					c.Set("pendingBalanceDeduction", true)
					c.Set("pendingDeductionAddress", walletAddress.(string))
					c.Set("pendingDeductionChain", chainStr)
					c.Set("pendingDeductionBalance", balance)
					c.Set("usingBalance", true)
					c.Header("X-RateLimit-Source", "balance")
					c.Header("X-Balance-Available", strconv.Itoa(balance))
					c.Next()
					return
				}

				// No balance either - show same message as anonymous
				windowEnd := rateLimit.WindowStart.Add(time.Duration(rl.cfg.RateLimitHours) * time.Hour)
				resetIn := time.Until(windowEnd)

				c.JSON(http.StatusTooManyRequests, models.ErrorResponse{
					Error:   "rate limit exceeded",
					Code:    "RATE_LIMIT_EXCEEDED",
					Details: formatRateLimitDetails(rateLimit.Count, rl.cfg.FreeCheckLimit, resetIn),
				})
				c.Abort()
				return
			}

			// Anonymous user - IP limit is final
			windowEnd := rateLimit.WindowStart.Add(time.Duration(rl.cfg.RateLimitHours) * time.Hour)
			resetIn := time.Until(windowEnd)

			c.JSON(http.StatusTooManyRequests, models.ErrorResponse{
				Error:   "rate limit exceeded",
				Code:    "RATE_LIMIT_EXCEEDED",
				Details: formatRateLimitDetails(rateLimit.Count, rl.cfg.FreeCheckLimit, resetIn),
			})
			c.Abort()
			return
		}

		// IP limit not exhausted - use IP-based check for everyone
		if err := rl.repo.IncrementRateLimit(ctx, ip, rateLimit.WindowStart); err != nil {
			c.Next()
			return
		}

		// For authenticated users with balance, set flag for handler to deduct after successful check
		if isAuthenticated && walletAddress != nil && chainStr != "" {
			balance, err := rl.repo.GetUserBalance(ctx, walletAddress.(string), chainStr)
			if err == nil && balance > 0 {
				c.Set("pendingBalanceDeduction", true)
				c.Set("pendingDeductionAddress", walletAddress.(string))
				c.Set("pendingDeductionChain", chainStr)
				c.Set("pendingDeductionBalance", balance)
				c.Set("usingBalance", true)
				c.Header("X-RateLimit-Source", "balance")
				c.Header("X-Balance-Available", strconv.Itoa(balance))
			}
		}

		// Update remaining header after increment
		remaining := max(0, rl.cfg.FreeCheckLimit-rateLimit.Count-1)
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Next()
	}
}

// setRateLimitHeaders sets common rate limit headers
func (rl *RateLimiter) setRateLimitHeaders(c *gin.Context, rateLimit *models.RateLimit, isUserAuthenticated bool) {
	limit := rl.cfg.FreeCheckLimit
	used := rateLimit.Count
	remaining := max(0, limit-used)
	windowEnd := rateLimit.WindowStart.Add(time.Duration(rl.cfg.RateLimitHours) * time.Hour)

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

	windowDuration := time.Duration(rl.cfg.RateLimitHours) * time.Hour
	if time.Since(rateLimit.WindowStart) > windowDuration {
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
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.ErrorResponse{
				Error: "invalid authorization header",
				Code:  "INVALID_TOKEN",
			})
			return
		}

		tokenString := parts[1]
		claims, err := authService.ValidateToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.ErrorResponse{
				Error: "invalid or expired token",
				Code:  "INVALID_TOKEN",
			})
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
