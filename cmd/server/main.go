package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"text/template"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/config"
	"vauln-address/internal/handlers"
	"vauln-address/internal/middleware"
	"vauln-address/internal/repository"
)

//go:embed ../../templates/*
var templateFS embed.FS

var tmpl *template.Template

type PageData struct {
	Title      string
	ActivePage string
	Content    string
}

func init() {
	var err error
	tmpl, err = template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}
}

func renderPage(c *gin.Context, pageTmpl, title, activePage string) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	tmpl.ExecuteTemplate(c.Writer, "base.html", PageData{
		Title:      title,
		ActivePage: activePage,
		Content:    pageTmpl,
	})
}

func renderErrorHTML(c *gin.Context, code int, title, message string) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(code)
	tmpl.ExecuteTemplate(c.Writer, "error.html", gin.H{
		"Code":        code,
		"Title":       title,
		"Message":     message,
		"Icon":        getErrorIcon(code),
		"ShowDetails": false,
	})
}

func renderErrorJSON(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{"error": message, "code": code})
}

func getErrorIcon(code int) string {
	switch code {
	case 400:
		return "📝"
	case 401:
		return "🔐"
	case 403:
		return "🚫"
	case 404:
		return "🔍"
	case 429:
		return "⏳"
	case 500:
		return "⚠️"
	default:
		return "❌"
	}
}

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
	h := handlers.New(repo)
	authService := h.GetAuthService()
	router.Use(middleware.AuthMiddleware(authService))

	// ========== PAGES (HTML) ==========
	router.GET("/", func(c *gin.Context) { renderPage(c, "home", "Home", "home") })
	router.GET("/roadmap", func(c *gin.Context) { renderPage(c, "roadmap", "Roadmap", "roadmap") })
	router.GET("/about", func(c *gin.Context) { renderPage(c, "about", "About", "about") })
	router.GET("/contact", func(c *gin.Context) { renderPage(c, "contact", "Contact", "contact") })
	router.GET("/support", func(c *gin.Context) { renderPage(c, "support", "Support", "support") })

	// 404 for pages (HTML)
	router.NoRoute(func(c *gin.Context) {
		renderErrorHTML(c, 404, "Page Not Found", "The page you're looking for doesn't exist or has been moved.")
	})

	// ========== API (JSON) ==========
	api := router.Group("/api")
	{
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
		api.GET("/orders/verify", h.VerifyPayment)
		api.POST("/check", rateLimiter.Limit(), h.CheckWallet)
		api.POST("/contact", h.SubmitContact)

		// 404 for API (JSON)
		api.NoRoute(func(c *gin.Context) {
			renderErrorJSON(c, 404, "Endpoint not found")
		})
	}

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
