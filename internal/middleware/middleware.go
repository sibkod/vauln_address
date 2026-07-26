package middleware

import (
	"context"
	"fmt"
	"net/http"
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

		// Check IP limit
		if rateLimit.Count >= rl.cfg.RateLimitRequests {
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

				// No balance either
				c.JSON(http.StatusPaymentRequired, models.ErrorResponse{
					Error:   "no checks remaining",
					Code:    "BALANCE_EXHAUSTED",
					Details: "IP limit exhausted. Please purchase more checks.",
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
				Details: formatRateLimitDetails(rateLimit.Count, rl.cfg.RateLimitRequests, resetIn),
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
		remaining := max(0, rl.cfg.RateLimitRequests-rateLimit.Count-1)
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Next()
	}
}

// setRateLimitHeaders sets common rate limit headers
func (rl *RateLimiter) setRateLimitHeaders(c *gin.Context, rateLimit *models.RateLimit, isUserAuthenticated bool) {
	limit := rl.cfg.RateLimitRequests
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
	return fmt.Sprintf("used %d of %d requests. Reset in %s", used, limit, formatDuration(resetIn))
}

func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%d minutes", minutes)
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
