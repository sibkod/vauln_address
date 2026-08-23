package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/config"
	"vauln-address/internal/handlers"
	"vauln-address/internal/middleware"
	"vauln-address/internal/repository"
	"vauln-address/internal/services"
)

func main() {
	cfg := config.Load()

	repo, err := repository.New(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer repo.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := repo.InitSchema(ctx); err != nil {
		log.Fatalf("Failed to initialize database schema: %v", err)
	}
	log.Println("Database schema initialized")

	// Initialize wallet stats table
	if err := repo.InitStatsTable(ctx); err != nil {
		log.Printf("Warning: failed to initialize stats table: %v", err)
	} else {
		log.Println("Wallet stats table initialized")
	}

	// Start price service for SOL/USD updates
	priceService := services.NewPriceService(cfg)
	defer priceService.Stop()

	// Start order service for expiring old orders
	orderService := services.NewOrderService(repo)
	orderService.Start()
	defer orderService.Stop()

	// Start rate limit reset service for daily reset at 00:00 UTC
	rateLimitResetService := services.NewRateLimitResetService(repo)
	rateLimitResetService.Start()
	defer rateLimitResetService.Stop()

	// Start report cleanup service - deletes anonymous reports after 24h
	reportCleanupService := services.NewReportCleanupService(repo)
	reportCleanupService.Start()
	defer reportCleanupService.Stop()

	// Start wallet import queue: a single background worker that writes
	// accumulated requests to the database in batched transactions
	walletQueue := services.NewWalletQueue(repo, cfg.WalletBatchSize,
		time.Duration(cfg.WalletFlushIntervalMs)*time.Millisecond, cfg.WalletQueueSize)
	walletQueue.Start()
	defer walletQueue.Stop()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORS())

	rateLimiter := middleware.NewRateLimiter(repo, cfg)
	h := handlers.New(repo, cfg, priceService, walletQueue)
	authService := h.GetAuthService()
	router.Use(middleware.AuthMiddleware(authService))

	// API routes
	api := router.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "vauln-address-api", "time": time.Now().UTC().Format(time.RFC3339)})
	})
	api.GET("/chains", h.GetSupportedChains)
	api.GET("/statuses", h.GetStatuses)
	api.GET("/recent", h.GetRecentChecks)
	api.GET("/checks", h.GetRecentChecks) // User check history
	api.GET("/pricing", h.GetPricing)
	api.GET("/packages", h.GetPackages) // Returns pre-defined pricing packages
	api.GET("/auth/nonce", h.GetNonce)
	api.POST("/auth/login", h.Authenticate)
	api.GET("/user/profile", middleware.RequireAuth(), h.GetUserProfile)
	api.GET("/user/balance", middleware.AuthMiddleware(authService), h.GetBalance) // Returns purchased balance for auth users, rate limit for anonymous
	api.GET("/me", middleware.AuthMiddleware(authService), h.GetMe)                // Comprehensive user info with balance and rate limits
	api.GET("/user/purchases", middleware.RequireAuth(), h.GetPurchaseHistory)
	api.POST("/orders", middleware.RequireAuth(), h.CreateOrder)
	api.POST("/orders/:id/cancel", middleware.RequireAuth(), h.CancelOrder)
	api.POST("/orders/:id/confirm", middleware.RequireAuth(), h.ConfirmOrder)
	api.GET("/orders/verify", middleware.RequireAuth(), h.VerifyPayment)
	api.POST("/payment/status/:signature", middleware.RequireAuth(), h.GetPaymentStatus)
	api.POST("/check", rateLimiter.Limit(), h.CheckWallet)
	api.GET("/report", h.GetReport)
	api.GET("/report/shared/:id", h.GetSharedReport)
	api.POST("/report/share", middleware.RequireAuth(), h.ShareReport)
	api.GET("/monitor/findings", h.GetMonitorFindings)
	api.GET("/monitor/stats", h.GetMonitorStats)
	api.GET("/captcha", h.GetCaptcha)
	api.POST("/drainer-reports", h.SubmitDrainerReport)
	api.POST("/bug-reports", h.SubmitBugReport)
	api.POST("/leak-reports", h.SubmitLeakReport)
	api.POST("/contact", h.SubmitContact)
	api.GET("/api-keys", middleware.RequireAuth(), h.ListAPIKeys)
	api.POST("/api-keys", middleware.RequireAuth(), h.CreateAPIKey)
	api.DELETE("/api-keys/:id", middleware.RequireAuth(), h.DeleteAPIKeyHandler)
	api.POST("/api-keys/revoke/:id", middleware.RequireAuth(), h.RevokeAPIKeyHandler)
	api.POST("/api-keys/renew", middleware.RequireAuth(), h.RenewAPIKeyHandler)

	// Admin routes (protected by admin API key)
	admin := api.Group("/admin")
	admin.Use(middleware.AdminMiddleware(cfg.AdminAPIKey))
	admin.POST("/wallets", h.AddWallet)
	admin.POST("/wallets/async", h.AddWalletAsync)
	admin.GET("/wallets/jobs/:id", h.GetWalletJob)
	admin.GET("/wallets", h.ListAdminWallets)
	admin.POST("/scanner/findings", h.IngestScanFinding)

	server := &http.Server{Addr: ":" + cfg.ServerPort, Handler: router, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}

	go func() {
		log.Printf("Starting server on port %s", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exited gracefully")
}
