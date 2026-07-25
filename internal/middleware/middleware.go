package middleware

import (
	"context"
	"net/http"
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
		_, isAuthenticated := c.Get("userID")

		// 1. Check IP rate limit for everyone
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
		} else if rateLimit.Count >= rl.cfg.RateLimitRequests {
			windowEnd := rateLimit.WindowStart.Add(time.Duration(rl.cfg.RateLimitHours) * time.Hour)
			resetIn := time.Until(windowEnd)

			c.JSON(http.StatusTooManyRequests, models.ErrorResponse{
				Error: "rate limit exceeded",
				Code:  "RATE_LIMIT_EXCEEDED",
				Details: formatRateLimitDetails(rateLimit.Count, rl.cfg.RateLimitRequests, resetIn),
			})
			c.Abort()
			return
		}

		// 2. For authenticated users: check their balance
		if isAuthenticated {
			userID := c.GetInt64("userID")
			user, err := rl.repo.GetUserByID(ctx, userID)
			if err != nil || user == nil {
				c.Next()
				return
			}

			// Check if user has checks remaining
			if user.Balance <= 0 {
				c.JSON(http.StatusPaymentRequired, models.ErrorResponse{
					Error: "no checks remaining",
					Code:  "BALANCE_EXHAUSTED",
					Details: "Your check package is exhausted. Please purchase more checks.",
				})
				c.Abort()
				return
			}

			// Deduct one check from balance
			if err := rl.repo.DeductUserBalance(ctx, userID, 1); err != nil {
				c.Next()
				return
			}

			// Pass remaining balance to context for handler
			c.Set("remainingBalance", user.Balance-1)
		} else {
			// For anonymous users: increment IP counter
			err = rl.repo.IncrementRateLimit(ctx, ip, rateLimit.WindowStart)
			if err != nil {
				c.Next()
				return
			}
		}

		c.Header("X-RateLimit-Limit", formatInt(rl.cfg.RateLimitRequests))
		if rateLimit != nil {
			c.Header("X-RateLimit-Remaining", formatInt(max(0, rl.cfg.RateLimitRequests-rateLimit.Count-1)))
		}
		c.Next()
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
	return "used " + formatInt(used) + " of " + formatInt(limit) +
		" requests. Reset in " + formatDuration(resetIn)
}

func formatInt(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0' + n/10)) + string(rune('0'+n%10))
}

func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours > 0 {
		return formatInt(hours) + "h " + formatInt(minutes) + "m"
	}
	return formatInt(minutes) + " minutes"
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
		c.Set("userID", claims.UserID)
		c.Set("userAddress", claims.Address)
		c.Next()
	}
}

// RequireAuth ensures the user is authenticated
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get("userID"); !exists {
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
