package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/models"
)

func postBulkCheck(t *testing.T, router *gin.Engine, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal bulk request: %v", err)
	}
	req := httptest.NewRequest("POST", "/api/check/bulk", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func setupBulkRouter(env *reportTestEnv) *gin.Engine {
	router := gin.New()
	router.POST("/api/check/bulk", env.handler.BulkCheckWallets)
	return router
}

// The bulk endpoint returns only the addresses present in the database,
// with their status, in the request order.
func TestBulkCheckWallets_FoundOnly(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()

	if _, err := env.repo.CreateWallet(ctx, scanAddrHacker, "solana", models.StatusHacker, "drainer operator", "solana_scan"); err != nil {
		t.Fatalf("CreateWallet hacker: %v", err)
	}
	if _, err := env.repo.CreateWallet(ctx, scanAddrVictim, "solana", models.StatusDrained, "", ""); err != nil {
		t.Fatalf("CreateWallet victim: %v", err)
	}

	router := setupBulkRouter(env)
	w := postBulkCheck(t, router, models.BulkCheckRequest{
		Chain: "solana",
		Addresses: []string{
			scanAddrSender1, // not in the database
			scanAddrVictim,
			scanAddrHacker,
			scanAddrVictim, // duplicate
			"not-an-address",
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp models.BulkCheckResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Chain != "solana" {
		t.Errorf("chain must echo the request, got %q", resp.Chain)
	}
	if resp.Checked != 3 {
		t.Errorf("3 valid unique addresses expected, got %d", resp.Checked)
	}
	if resp.Found != 2 || len(resp.Results) != 2 {
		t.Fatalf("only the 2 known addresses must be returned, got %+v", resp.Results)
	}
	if resp.Results[0].Address != scanAddrVictim || resp.Results[0].Status != string(models.StatusDrained) {
		t.Errorf("first result must be the drained victim in request order, got %+v", resp.Results[0])
	}
	if resp.Results[1].Address != scanAddrHacker || resp.Results[1].Status != string(models.StatusHacker) {
		t.Errorf("second result must be the hacker, got %+v", resp.Results[1])
	}
	if resp.Results[1].Reason != "drainer operator" || resp.Results[1].Source != "solana_scan" {
		t.Errorf("registry metadata must be included, got %+v", resp.Results[1])
	}
}

// Wallets whose status is outside the reported threat subset (scam, mixer,
// sanctioned, frozen, vulnerable, safe, exchange, unknown) are not
// disclosed by the bulk endpoint.
func TestBulkCheckWallets_FiltersNonThreatStatuses(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()

	statuses := []models.WalletStatus{
		models.StatusScam, models.StatusMixer, models.StatusSanctioned,
		models.StatusFrozen, models.StatusVulnerable, models.StatusSafe,
		models.StatusExchange, models.StatusUnknown,
	}
	addrs := make([]string, 0, len(statuses)+1)
	for i, st := range statuses {
		addr := fmt.Sprintf("0x%040x", i+1)
		if _, err := env.repo.CreateWallet(ctx, addr, "evm", st, "", ""); err != nil {
			t.Fatalf("CreateWallet %s: %v", st, err)
		}
		addrs = append(addrs, addr)
	}
	const threat = "0x00000000000000000000000000000000000000ff"
	if _, err := env.repo.CreateWallet(ctx, threat, "evm", models.StatusSuspicious, "", ""); err != nil {
		t.Fatalf("CreateWallet threat: %v", err)
	}
	addrs = append(addrs, threat)

	router := setupBulkRouter(env)
	w := postBulkCheck(t, router, models.BulkCheckRequest{Chain: "evm", Addresses: addrs})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp models.BulkCheckResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Found != 1 || len(resp.Results) != 1 || resp.Results[0].Address != threat {
		t.Fatalf("only the suspicious wallet must be reported, got %+v", resp.Results)
	}
}

// A specific EVM network name is accepted and normalized to the canonical
// evm chain; the address match is case-insensitive.
func TestBulkCheckWallets_EVMNetworkNormalized(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()

	const evmAddr = "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1"
	if _, err := env.repo.CreateWallet(ctx, evmAddr, "evm", models.StatusPhishing, "", ""); err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}

	router := setupBulkRouter(env)
	w := postBulkCheck(t, router, models.BulkCheckRequest{
		Chain:     "bnb",
		Addresses: []string{strings.ToLower(evmAddr)},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp models.BulkCheckResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Chain != "evm" {
		t.Errorf("specific network must normalize to evm, got %q", resp.Chain)
	}
	if resp.Found != 1 || resp.Results[0].Status != string(models.StatusPhishing) {
		t.Fatalf("case-insensitive EVM match must find the wallet, got %+v", resp)
	}
}

// Validation errors: unknown chain, empty batch, oversized batch.
func TestBulkCheckWallets_Validation(t *testing.T) {
	env := setupReportTest(t)
	router := setupBulkRouter(env)

	w := postBulkCheck(t, router, models.BulkCheckRequest{Chain: "dogecoin", Addresses: []string{"x"}})
	if w.Code != http.StatusBadRequest {
		t.Errorf("unknown chain must be rejected, got %d", w.Code)
	}

	w = postBulkCheck(t, router, models.BulkCheckRequest{Chain: "btc", Addresses: []string{}})
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty batch must be rejected, got %d", w.Code)
	}

	tooMany := make([]string, bulkCheckMaxAddresses+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("addr-%d", i)
	}
	w = postBulkCheck(t, router, models.BulkCheckRequest{Chain: "btc", Addresses: tooMany})
	if w.Code != http.StatusBadRequest {
		t.Errorf("oversized batch must be rejected, got %d", w.Code)
	}
}
