package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vauln-address/internal/models"
)

// GET /api/admin/wallets?status=... — scanner seeds its hacker watch set.
func TestListAdminWallets(t *testing.T) {
	env := setupReportTest(t)
	router := setupScannerRouter(env)
	ctx := context.Background()

	if _, err := env.repo.CreateWallet(ctx, scanAddrHacker, "solana", models.StatusHacker, "t", "test"); err != nil {
		t.Fatalf("seed hacker: %v", err)
	}
	if _, err := env.repo.CreateWallet(ctx, scanAddrVictim, "solana", models.StatusDrained, "t", "test"); err != nil {
		t.Fatalf("seed drained: %v", err)
	}

	do := func(path, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		if key != "" {
			req.Header.Set("X-Admin-Key", key)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	if w := do("/api/admin/wallets?status=hacker&chain=solana", "wrong-key"); w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without admin key, got %d", w.Code)
	}

	w := do("/api/admin/wallets?status=hacker&chain=solana", scanTestAdminKey)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count     int      `json:"count"`
		Addresses []string `json:"addresses"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Count != 1 || len(resp.Addresses) != 1 || resp.Addresses[0] != scanAddrHacker {
		t.Fatalf("expected only the hacker wallet, got %+v", resp)
	}

	// comma-separated statuses in a single param
	w = do("/api/admin/wallets?status=hacker,drained&chain=solana", scanTestAdminKey)
	if w.Code != http.StatusOK {
		t.Fatalf("comma statuses: got %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode comma response: %v", err)
	}
	if resp.Count != 2 {
		t.Fatalf("expected 2 wallets for hacker,drained, got %+v", resp)
	}

	if w := do("/api/admin/wallets?status=bogus&chain=solana", scanTestAdminKey); w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid status, got %d", w.Code)
	}
	if w := do("/api/admin/wallets?chain=solana", scanTestAdminKey); w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without status, got %d", w.Code)
	}
}
