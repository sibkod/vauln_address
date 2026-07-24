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

	router := gin.Default()

	router.Use(middleware.CORS())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "vauln-address-api",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})

	rateLimiter := middleware.NewRateLimiter(repo, cfg)
	h := handlers.New(repo)

	// Initialize auth service
	authService := h.GetAuthService()

	// Auth middleware for optional authentication
	router.Use(middleware.AuthMiddleware(authService))

	// Public routes
	router.GET("/", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.File("index.html")
	})

	api := router.Group("")
	{
		// Public endpoints
		api.GET("/chains", h.GetSupportedChains)
		api.GET("/recent", h.GetRecentChecks)
		api.GET("/pricing", h.GetPricing)

		// Auth endpoints
		api.GET("/auth/nonce", h.GetNonce)
		api.POST("/auth/login", h.Authenticate)
		
		// Protected endpoints
		api.GET("/user/profile", middleware.RequireAuth(), h.GetUserProfile)
		api.POST("/orders", middleware.RequireAuth(), h.CreateOrder)
		api.GET("/orders/verify", h.VerifyPayment)

		// Check endpoint (rate limited, auth optional)
		api.POST("/check", rateLimiter.Limit(), h.CheckWallet)

		api.POST("/contact", h.SubmitContact)
	}

	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

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
