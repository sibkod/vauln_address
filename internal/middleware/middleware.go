package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

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
			c.Next()
			return
		}

		if rateLimit.Count >= rl.cfg.RateLimitRequests {
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

		err = rl.repo.IncrementRateLimit(ctx, ip, rateLimit.WindowStart)
		if err != nil {
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", formatInt(rl.cfg.RateLimitRequests))
		c.Header("X-RateLimit-Remaining", formatInt(rl.cfg.RateLimitRequests-rateLimit.Count-1))
		c.Next()
	}
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
