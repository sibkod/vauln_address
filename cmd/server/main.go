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

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORS())

	rateLimiter := middleware.NewRateLimiter(repo, cfg)
	h := handlers.New(repo, cfg)
	authService := h.GetAuthService()
	router.Use(middleware.AuthMiddleware(authService))

	// Serve React frontend from frontend/dist
	frontendDist := "../frontend/dist"
	if _, err := os.Stat(frontendDist); err == nil {
		router.Static("/assets", frontendDist+"/assets")
		router.GET("/", func(c *gin.Context) {
			c.File(frontendDist + "/index.html")
		})
		router.NoRoute(func(c *gin.Context) {
			c.File(frontendDist + "/index.html")
		})
		log.Println("Serving React frontend from:", frontendDist)
	} else {
		log.Fatal("frontend/dist not found! Run 'npm run build' in frontend/")
	}

	// API routes
	api := router.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "vauln-address-api", "time": time.Now().UTC().Format(time.RFC3339)})
	})
	api.GET("/chains", h.GetSupportedChains)
	api.GET("/recent", h.GetRecentChecks)
	api.GET("/pricing", h.GetPricing)
	api.GET("/auth/nonce", h.GetNonce)
	api.POST("/auth/login", h.Authenticate)
	api.GET("/user/profile", middleware.RequireAuth(), h.GetUserProfile)
	api.POST("/orders", middleware.RequireAuth(), h.CreateOrder)
	api.POST("/orders/:id/confirm", middleware.RequireAuth(), h.ConfirmOrder)
	api.GET("/orders/verify", middleware.RequireAuth(), h.VerifyPayment)
	api.POST("/payment/status/:signature", middleware.RequireAuth(), h.GetPaymentStatus)
	api.POST("/check", rateLimiter.Limit(), h.CheckWallet)
	api.POST("/contact", h.SubmitContact)
	api.GET("/api-keys", middleware.RequireAuth(), h.ListAPIKeys)
	api.POST("/api-keys", middleware.RequireAuth(), h.CreateAPIKey)
	api.DELETE("/api-keys/:id", middleware.RequireAuth(), h.DeleteAPIKeyHandler)
	api.POST("/api-keys/revoke/:id", middleware.RequireAuth(), h.RevokeAPIKeyHandler)
	api.POST("/api-keys/renew", middleware.RequireAuth(), h.RenewAPIKeyHandler)

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
