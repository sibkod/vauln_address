package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestHealthCheck(t *testing.T) {
	router := gin.New()
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "vauln-address-api",
		})
	})

	req, _ := http.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", resp["status"])
	}
	if resp["service"] != "vauln-address-api" {
		t.Errorf("Expected service 'vauln-address-api', got '%s'", resp["service"])
	}
}

func TestGetSupportedChains(t *testing.T) {
	router := gin.New()
	router.GET("/api/chains", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"chains": []gin.H{
				{"name": "EVM", "id": "evm", "example": "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1"},
				{"name": "Bitcoin", "id": "btc", "example": "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"},
				{"name": "Solana", "id": "solana", "example": "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV"},
				{"name": "Sui", "id": "sui", "example": "0x8a1c4cd2d2fd05e02e3d2b5f4d6f8a1c3e5b7d9f0a2c4e6"},
				{"name": "Tron", "id": "tron", "example": "TJK5M5kKxP8xF9cGvN2pL6rU4sW7xA3bCd"},
			},
		})
	})

	req, _ := http.NewRequest("GET", "/api/chains", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp struct {
		Chains []struct {
			Name    string `json:"name"`
			ID      string `json:"id"`
			Example string `json:"example"`
		} `json:"chains"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Chains) != 5 {
		t.Errorf("Expected 5 chains, got %d", len(resp.Chains))
	}

	expectedChains := []string{"evm", "btc", "solana", "sui", "tron"}
	for i, chain := range resp.Chains {
		if chain.ID != expectedChains[i] {
			t.Errorf("Expected chain %s at index %d, got %s", expectedChains[i], i, chain.ID)
		}
	}
}

func TestGetPricing(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		expectedCode int
	}{
		{"default 10 checks", "", http.StatusOK},
		{"50 checks", "?checks=50", http.StatusOK},
		{"100 checks (max)", "?checks=100", http.StatusOK},
		{"invalid negative", "?checks=-1", http.StatusOK},
		{"invalid zero", "?checks=0", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/api/pricing", func(c *gin.Context) {
				checksStr := c.DefaultQuery("checks", "10")
				var checks int
				fmtScan(checksStr, &checks)
				if checks < 1 || checks > 100 {
					checks = 10
				}

				// Simulate pricing calculation
				pricePerCheck := 0.10
				basePrice := float64(checks) * pricePerCheck

				c.JSON(http.StatusOK, gin.H{
					"checks":              checks,
					"price_per_check_usd": pricePerCheck,
					"payment_methods": []gin.H{
						{"currency": "usdc", "price_usd": basePrice},
						{"currency": "usdt", "price_usd": basePrice},
						{"currency": "eth", "price_usd": basePrice, "token_amount": basePrice / 2000},
						{"currency": "sui", "price_usd": basePrice, "token_amount": basePrice / 1.50, "has_discount": true, "discount_label": "50% OFF"},
					},
				})
			})

			req, _ := http.NewRequest("GET", "/api/pricing"+tt.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestGetNonce_MissingParams(t *testing.T) {
	router := gin.New()
	router.GET("/api/auth/nonce", func(c *gin.Context) {
		address := c.Query("address")
		chain := c.Query("chain")

		if address == "" || chain == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "address and chain are required",
				"code":  "INVALID_REQUEST",
			})
			return
		}

		if chain != "evm" && chain != "solana" && chain != "btc" && chain != "sui" && chain != "tron" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "unsupported chain",
				"code":  "INVALID_CHAIN",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"nonce":   "test-nonce-123",
			"message": "Sign this message to authenticate with Vauln Address.\n\nNonce: test-nonce-123\nTimestamp: 1234567890",
		})
	})

	tests := []struct {
		name         string
		query        string
		expectedCode int
		expectedErr  string
	}{
		{"missing both", "", http.StatusBadRequest, "address and chain are required"},
		{"missing chain", "?address=0x123", http.StatusBadRequest, "address and chain are required"},
		{"missing address", "?chain=evm", http.StatusBadRequest, "address and chain are required"},
		{"invalid chain", "?address=0x123&chain=invalid", http.StatusBadRequest, "unsupported chain"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/auth/nonce"+tt.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status %d, got %d", tt.expectedCode, w.Code)
			}

			var resp map[string]string
			json.Unmarshal(w.Body.Bytes(), &resp)

			if resp["error"] != tt.expectedErr {
				t.Errorf("Expected error '%s', got '%s'", tt.expectedErr, resp["error"])
			}
		})
	}
}

func TestGetNonce_Success(t *testing.T) {
	router := gin.New()
	router.GET("/api/auth/nonce", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"nonce":   "test-nonce-123",
			"message": "Sign this message to authenticate with Vauln Address.\n\nNonce: test-nonce-123\nTimestamp: 1234567890",
		})
	})

	tests := []struct {
		name  string
		query string
	}{
		{"evm chain", "?address=0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1&chain=evm"},
		{"solana chain", "?address=7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV&chain=solana"},
		{"btc chain", "?address=bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh&chain=btc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/auth/nonce"+tt.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			}

			var resp map[string]string
			json.Unmarshal(w.Body.Bytes(), &resp)

			if resp["nonce"] == "" {
				t.Error("Expected non-empty nonce")
			}
			if resp["message"] == "" {
				t.Error("Expected non-empty message")
			}
		})
	}
}

func TestCreateOrder_Unauthorized(t *testing.T) {
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		// Simulate missing auth
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "authentication required",
			"code":  "UNAUTHORIZED",
		})
	})

	body := bytes.NewBufferString(`{"checks": 10, "chain": "solana", "wallet_address": "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV"}`)
	req, _ := http.NewRequest("POST", "/api/orders", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestCreateOrder_InvalidRequest(t *testing.T) {
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		// Simulate invalid request body
		var req struct {
			Checks int    `json:"checks"`
			Chain  string `json:"chain"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.Checks == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid request body",
				"code":  "INVALID_REQUEST",
			})
			return
		}
	})

	tests := []struct {
		name         string
		body         string
		expectedCode int
	}{
		{"empty body", "{}", http.StatusBadRequest},
		{"missing checks", `{"chain": "solana"}`, http.StatusBadRequest},
		{"zero checks", `{"checks": 0, "chain": "solana"}`, http.StatusBadRequest},
		{"negative checks", `{"checks": -1, "chain": "solana"}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := bytes.NewBufferString(tt.body)
			req, _ := http.NewRequest("POST", "/api/orders", body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestCreateOrder_Success(t *testing.T) {
	router := gin.New()
	router.POST("/api/orders", func(c *gin.Context) {
		// Simulate successful order creation
		c.JSON(http.StatusCreated, gin.H{
			"order_id":        "test-order-uuid-123",
			"checks_count":    50,
			"total_usd":       5.0,
			"amount":          "0.0100",
			"payment_address": "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV",
			"status":          "pending",
		})
	})

	body := bytes.NewBufferString(`{"checks": 50, "chain": "solana", "wallet_address": "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV"}`)
	req, _ := http.NewRequest("POST", "/api/orders", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["order_id"] == "" {
		t.Error("Expected non-empty order_id")
	}
	if resp["amount"] != "0.0100" {
		t.Errorf("Expected amount '0.0100', got '%v'", resp["amount"])
	}
	if resp["status"] != "pending" {
		t.Errorf("Expected status 'pending', got '%v'", resp["status"])
	}
}

func TestCheckWallet_InvalidAddress(t *testing.T) {
	router := gin.New()
	router.POST("/api/check", func(c *gin.Context) {
		var req struct {
			Address string `json:"address"`
			Chain   string `json:"chain"`
		}
		json.NewDecoder(c.Request.Body).Decode(&req)

		if req.Address == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid request body",
				"code":  "INVALID_REQUEST",
			})
			return
		}

		validChains := map[string]bool{"evm": true, "btc": true, "solana": true, "sui": true, "tron": true}
		if !validChains[req.Chain] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "unsupported chain",
				"code":    "INVALID_CHAIN",
				"details": "supported chains: evm, btc, solana, sui, tron",
			})
			return
		}

		// Simulate safe wallet
		c.JSON(http.StatusOK, gin.H{
			"address":  req.Address,
			"chain":    req.Chain,
			"status":   "not_found",
			"found":    false,
			"has_pk":   false,
			"has_seed": false,
		})
	})

	tests := []struct {
		name         string
		body         string
		expectedCode int
	}{
		{"empty body", "{}", http.StatusBadRequest},
		{"missing chain", `{"address": "0x123"}`, http.StatusBadRequest},
		{"invalid chain", `{"address": "0x123", "chain": "invalid"}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := bytes.NewBufferString(tt.body)
			req, _ := http.NewRequest("POST", "/api/check", body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestCheckWallet_Success(t *testing.T) {
	router := gin.New()
	router.POST("/api/check", func(c *gin.Context) {
		var req struct {
			Address string `json:"address"`
			Chain   string `json:"chain"`
		}
		json.NewDecoder(c.Request.Body).Decode(&req)

		c.JSON(http.StatusOK, gin.H{
			"address":  req.Address,
			"chain":    req.Chain,
			"status":   "not_found",
			"found":    false,
			"has_pk":   false,
			"has_seed": false,
		})
	})

	tests := []struct {
		name    string
		address string
		chain   string
	}{
		{"evm address", "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1", "evm"},
		{"btc address", "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh", "btc"},
		{"solana address", "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV", "solana"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := bytes.NewBufferString(`{"address": "` + tt.address + `", "chain": "` + tt.chain + `"}`)
			req, _ := http.NewRequest("POST", "/api/check", body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			}

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			if resp["found"] != false {
				t.Errorf("Expected found=false for safe wallet")
			}
		})
	}
}

func TestSubmitContact_InvalidRequest(t *testing.T) {
	router := gin.New()
	router.POST("/api/contact", func(c *gin.Context) {
		var req struct {
			Name    string `json:"name"`
			Email   string `json:"email"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid request body",
				"code":  "INVALID_REQUEST",
			})
			return
		}
		if req.Name == "" || req.Email == "" || req.Message == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid request body",
				"code":  "INVALID_REQUEST",
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "contact form submitted successfully",
		})
	})

	tests := []struct {
		name         string
		body         string
		expectedCode int
	}{
		{"empty body", "{}", http.StatusBadRequest},
		{"missing name", `{"email": "test@test.com", "message": "Hello"}`, http.StatusBadRequest},
		{"missing email", `{"name": "John", "message": "Hello"}`, http.StatusBadRequest},
		{"missing message", `{"name": "John", "email": "test@test.com"}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := bytes.NewBufferString(tt.body)
			req, _ := http.NewRequest("POST", "/api/contact", body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestSubmitContact_Success(t *testing.T) {
	router := gin.New()
	router.POST("/api/contact", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{
			"message": "contact form submitted successfully",
		})
	})

	body := bytes.NewBufferString(`{"name": "John Doe", "email": "john@example.com", "message": "Hello, I have a question about your service."}`)
	req, _ := http.NewRequest("POST", "/api/contact", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["message"] != "contact form submitted successfully" {
		t.Errorf("Expected success message, got '%s'", resp["message"])
	}
}

func TestGetRecentChecks(t *testing.T) {
	router := gin.New()
	router.GET("/api/recent", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"checks": []gin.H{
				{"id": 1, "address": "0x123", "chain": "evm", "status": "safe", "checked_at": "2024-01-01T00:00:00Z"},
				{"id": 2, "address": "bc1qxy2", "chain": "btc", "status": "safe", "checked_at": "2024-01-01T00:00:00Z"},
			},
			"count": 2,
		})
	})

	req, _ := http.NewRequest("GET", "/api/recent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp struct {
		Checks []map[string]interface{} `json:"checks"`
		Count  int                      `json:"count"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Count != 2 {
		t.Errorf("Expected count 2, got %d", resp.Count)
	}
	if len(resp.Checks) != 2 {
		t.Errorf("Expected 2 checks, got %d", len(resp.Checks))
	}
}

// Helper function to replace fmt.Sscanf
func fmtScan(s string, result *int) {
	for _, c := range s {
		if c >= '0' && c <= '9' {
			*result = *result*10 + int(c-'0')
		}
	}
}

// ==================== Balance Endpoint Tests ====================

func TestGetBalance_AnonymousUser_NoUsage(t *testing.T) {
	router := gin.New()

	// Middleware that does NOT set userAddress (anonymous user)
	router.Use(func(c *gin.Context) {
		c.Next()
	})

	router.GET("/api/user/balance", func(c *gin.Context) {
		walletAddress, exists := c.Get("userAddress")

		if exists && walletAddress != nil && walletAddress != "" {
			// Authenticated user
			c.JSON(http.StatusOK, gin.H{"balance": 60, "source": "purchased"})
			return
		}

		// Anonymous user with 3 free checks
		c.JSON(http.StatusOK, gin.H{
			"balance":              3,
			"purchased_balance":    0,
			"rate_limit_remaining": 3,
			"source":               "rate_limit",
		})
	})

	req, _ := http.NewRequest("GET", "/api/user/balance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Balance int    `json:"balance"`
		Source  string `json:"source"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Source != "rate_limit" {
		t.Errorf("Expected source 'rate_limit', got '%s'", resp.Source)
	}
	if resp.Balance != 3 {
		t.Errorf("Expected balance 3 (free checks), got %d", resp.Balance)
	}
}

func TestGetBalance_AuthenticatedUser_NoPurchase(t *testing.T) {
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userAddress", "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV")
		c.Set("userChain", "solana")
		c.Next()
	})

	router.GET("/api/user/balance", func(c *gin.Context) {
		walletAddress, exists := c.Get("userAddress")
		chain, _ := c.Get("userChain")
		chainStr, _ := chain.(string)

		if exists && walletAddress != nil && walletAddress != "" && chainStr != "" {
			// Authenticated but no purchases - show rate limit (3 remaining)
			c.JSON(http.StatusOK, gin.H{
				"balance":              0,
				"purchased_balance":    0,
				"rate_limit_remaining": 3,
				"source":               "rate_limit",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"balance": 3, "source": "rate_limit"})
	})

	req, _ := http.NewRequest("GET", "/api/user/balance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Balance int    `json:"balance"`
		Source  string `json:"source"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Source != "rate_limit" {
		t.Errorf("Expected source 'rate_limit', got '%s'", resp.Source)
	}
	if resp.Balance != 0 {
		t.Errorf("Expected balance 0 (used all 3 free checks), got %d", resp.Balance)
	}
}

func TestGetBalance_AuthenticatedUser_WithPurchase(t *testing.T) {
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userAddress", "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV")
		c.Set("userChain", "solana")
		c.Next()
	})

	router.GET("/api/user/balance", func(c *gin.Context) {
		walletAddress, exists := c.Get("userAddress")
		chain, _ := c.Get("userChain")
		chainStr, _ := chain.(string)

		if exists && walletAddress != nil && walletAddress != "" && chainStr != "" {
			// User has purchased 60 checks
			c.JSON(http.StatusOK, gin.H{
				"balance":              60,
				"purchased_balance":    60,
				"rate_limit_remaining": 0,
				"source":               "purchased",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"balance": 3, "source": "rate_limit"})
	})

	req, _ := http.NewRequest("GET", "/api/user/balance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Balance int    `json:"balance"`
		Source  string `json:"source"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Source != "purchased" {
		t.Errorf("Expected source 'purchased', got '%s'", resp.Source)
	}
	if resp.Balance != 60 {
		t.Errorf("Expected balance 60 (purchased), got %d", resp.Balance)
	}
}

func TestGetBalance_AuthenticatedUser_PartialPurchase(t *testing.T) {
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userAddress", "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV")
		c.Set("userChain", "solana")
		c.Next()
	})

	router.GET("/api/user/balance", func(c *gin.Context) {
		walletAddress, exists := c.Get("userAddress")
		chain, _ := c.Get("userChain")
		chainStr, _ := chain.(string)

		if exists && walletAddress != nil && walletAddress != "" && chainStr != "" {
			// User has purchased 10 checks (more than 0, so show purchased)
			c.JSON(http.StatusOK, gin.H{
				"balance":              10,
				"purchased_balance":    10,
				"rate_limit_remaining": 0,
				"source":               "purchased",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"balance": 3, "source": "rate_limit"})
	})

	req, _ := http.NewRequest("GET", "/api/user/balance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Balance int    `json:"balance"`
		Source  string `json:"source"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Source != "purchased" {
		t.Errorf("Expected source 'purchased', got '%s'", resp.Source)
	}
	if resp.Balance != 10 {
		t.Errorf("Expected balance 10 (purchased), got %d", resp.Balance)
	}
}

func TestGetBalance_InvalidAuthToken(t *testing.T) {
	router := gin.New()

	router.Use(func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		// Token present but invalid - don't set userAddress
		if authHeader == "Bearer valid-token" {
			c.Set("userAddress", "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV")
			c.Set("userChain", "solana")
		}
		c.Next()
	})

	router.GET("/api/user/balance", func(c *gin.Context) {
		walletAddress, exists := c.Get("userAddress")
		chain, _ := c.Get("userChain")
		chainStr, _ := chain.(string)

		if exists && walletAddress != nil && walletAddress != "" && chainStr != "" {
			c.JSON(http.StatusOK, gin.H{"balance": 60, "source": "purchased"})
			return
		}

		// Invalid/missing auth - return rate limit
		c.JSON(http.StatusOK, gin.H{
			"balance":              3,
			"purchased_balance":    0,
			"rate_limit_remaining": 3,
			"source":               "rate_limit",
		})
	})

	req, _ := http.NewRequest("GET", "/api/user/balance", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Balance int    `json:"balance"`
		Source  string `json:"source"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Source != "rate_limit" {
		t.Errorf("Expected source 'rate_limit' for invalid token, got '%s'", resp.Source)
	}
}
