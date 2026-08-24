package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/models"
	"vauln-address/internal/services"
)

// paymentTestRPC serves a canned getTransaction response.
func paymentTestRPC(t *testing.T, feePayer, recipient string, paidLamports uint64, blockTime int64, metaErr any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"slot":      12345,
				"blockTime": blockTime,
				"meta": map[string]interface{}{
					"err":          metaErr,
					"fee":          5000,
					"preBalances":  []uint64{10_000_000_000, 0},
					"postBalances": []uint64{10_000_000_000 - paidLamports - 5000, paidLamports},
				},
				"transaction": map[string]interface{}{
					"message": map[string]interface{}{
						"accountKeys": []map[string]interface{}{
							{"pubkey": feePayer},
							{"pubkey": recipient},
						},
						"instructions": []interface{}{},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// paymentTestRouter mounts the payment endpoints with the authenticated
// wallet injected into the gin context (as RequireAuth would).
func paymentTestRouter(env *reportTestEnv, wallet string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userAddress", wallet)
		c.Set("userChain", "solana")
		c.Next()
	})
	router.POST("/orders/:id/confirm", env.handler.ConfirmOrder)
	router.GET("/orders/verify", env.handler.VerifyPayment)
	router.POST("/payment/status/:signature", env.handler.GetPaymentStatus)
	return router
}

func TestConfirmOrder_VerifiedPayment(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()

	wallet := "BuyerWallet111"
	if err := env.repo.UpsertUserNonce(wallet, "solana", "nonce-"+wallet); err != nil {
		t.Fatalf("create user: %v", err)
	}
	paymentAddr := "MerchantAddr111"
	order, err := env.repo.CreateOrder(ctx, wallet, "solana", 10, 9.99, "solana", 0.5, paymentAddr)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	env.handler.paymentVerifier = services.NewPaymentVerifier(
		paymentTestRPC(t, wallet, paymentAddr, 500_000_000, time.Now().Unix(), nil).URL)

	router := paymentTestRouter(env, wallet)
	req := httptest.NewRequest("POST", "/orders/"+order.OrderUUID+"/confirm?tx_signature=sig-paid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	updated, _ := env.repo.GetOrderByUUID(ctx, order.OrderUUID)
	if updated.Status != string(models.PaymentCompleted) {
		t.Fatalf("order must be completed, got %q", updated.Status)
	}
	user, _ := env.repo.GetUserByWallet(ctx, wallet, "solana")
	if user.Balance != 13 {
		t.Fatalf("expected balance 13 (3 free + 10 paid), got %d", user.Balance)
	}
}

func TestConfirmOrder_RejectedTxFailsOrder(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()

	wallet := "BuyerWallet222"
	if err := env.repo.UpsertUserNonce(wallet, "solana", "nonce-"+wallet); err != nil {
		t.Fatalf("create user: %v", err)
	}
	order, err := env.repo.CreateOrder(ctx, wallet, "solana", 10, 9.99, "solana", 0.5, "MerchantAddr222")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	// tx reverted on-chain
	env.handler.paymentVerifier = services.NewPaymentVerifier(
		paymentTestRPC(t, wallet, "MerchantAddr222", 500_000_000, time.Now().Unix(),
			map[string]interface{}{"InstructionError": []interface{}{0, "Custom"}}).URL)

	router := paymentTestRouter(env, wallet)
	req := httptest.NewRequest("POST", "/orders/"+order.OrderUUID+"/confirm?tx_signature=sig-reverted", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "TX_FAILED") {
		t.Fatalf("expected TX_FAILED code, got %s", w.Body.String())
	}
	updated, _ := env.repo.GetOrderByUUID(ctx, order.OrderUUID)
	if updated.Status != string(models.PaymentFailed) {
		t.Fatalf("order must be failed, got %q", updated.Status)
	}
	user, _ := env.repo.GetUserByWallet(ctx, wallet, "solana")
	if user.Balance != 3 {
		t.Fatalf("no paid balance may be credited for a failed tx, got %d", user.Balance)
	}
}

func TestConfirmOrder_WrongRecipient(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()

	wallet := "BuyerWallet333"
	if err := env.repo.UpsertUserNonce(wallet, "solana", "nonce-"+wallet); err != nil {
		t.Fatalf("create user: %v", err)
	}
	order, err := env.repo.CreateOrder(ctx, wallet, "solana", 5, 4.99, "solana", 0.25, "MerchantAddr333")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	// tx pays someone else entirely
	env.handler.paymentVerifier = services.NewPaymentVerifier(
		paymentTestRPC(t, wallet, "AttackerAddr", 250_000_000, time.Now().Unix(), nil).URL)

	router := paymentTestRouter(env, wallet)
	req := httptest.NewRequest("POST", "/orders/"+order.OrderUUID+"/confirm?tx_signature=sig-wrong-rcpt", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "AMOUNT_MISMATCH") {
		t.Fatalf("expected 400 AMOUNT_MISMATCH, got %d: %s", w.Code, w.Body.String())
	}
	user, _ := env.repo.GetUserByWallet(ctx, wallet, "solana")
	if user.Balance != 3 {
		t.Fatalf("no paid balance may be credited, got %d", user.Balance)
	}
}

func TestConfirmOrder_Underpaid(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()

	wallet := "BuyerWallet444"
	if err := env.repo.UpsertUserNonce(wallet, "solana", "nonce-"+wallet); err != nil {
		t.Fatalf("create user: %v", err)
	}
	order, err := env.repo.CreateOrder(ctx, wallet, "solana", 5, 4.99, "solana", 0.5, "MerchantAddr444")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	// tx pays only 0.1 SOL instead of the required 0.5
	env.handler.paymentVerifier = services.NewPaymentVerifier(
		paymentTestRPC(t, wallet, "MerchantAddr444", 100_000_000, time.Now().Unix(), nil).URL)

	router := paymentTestRouter(env, wallet)
	req := httptest.NewRequest("POST", "/orders/"+order.OrderUUID+"/confirm?tx_signature=sig-underpaid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "AMOUNT_MISMATCH") {
		t.Fatalf("expected 400 AMOUNT_MISMATCH, got %d: %s", w.Code, w.Body.String())
	}
	updated, _ := env.repo.GetOrderByUUID(ctx, order.OrderUUID)
	if updated.Status != string(models.PaymentFailed) {
		t.Fatalf("underpaid order must be failed, got %q", updated.Status)
	}
}

func TestConfirmOrder_ReplayedTx(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()

	wallet := "BuyerWallet555"
	if err := env.repo.UpsertUserNonce(wallet, "solana", "nonce-"+wallet); err != nil {
		t.Fatalf("create user: %v", err)
	}
	paymentAddr := "MerchantAddr555"
	order1, err := env.repo.CreateOrder(ctx, wallet, "solana", 10, 9.99, "solana", 0.5, paymentAddr)
	if err != nil {
		t.Fatalf("create order1: %v", err)
	}
	order2, err := env.repo.CreateOrder(ctx, wallet, "solana", 10, 9.99, "solana", 0.5, paymentAddr)
	if err != nil {
		t.Fatalf("create order2: %v", err)
	}

	env.handler.paymentVerifier = services.NewPaymentVerifier(
		paymentTestRPC(t, wallet, paymentAddr, 500_000_000, time.Now().Unix(), nil).URL)
	router := paymentTestRouter(env, wallet)

	// first order completes with the tx
	req := httptest.NewRequest("POST", "/orders/"+order1.OrderUUID+"/confirm?tx_signature=sig-replay", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first confirm must succeed, got %d: %s", w.Code, w.Body.String())
	}

	// second order must not be payable with the same tx
	req = httptest.NewRequest("POST", "/orders/"+order2.OrderUUID+"/confirm?tx_signature=sig-replay", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("replayed tx must be rejected with 409, got %d: %s", w.Code, w.Body.String())
	}
	user, _ := env.repo.GetUserByWallet(ctx, wallet, "solana")
	if user.Balance != 13 {
		t.Fatalf("paid balance must be credited exactly once, got %d", user.Balance)
	}
}

func TestVerifyPayment_RequiresOwnership(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()

	order, err := env.repo.CreateOrder(ctx, "OwnerWallet", "solana", 10, 9.99, "solana", 0.5, "MerchantAddr666")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	router := paymentTestRouter(env, "OtherWallet")
	req := httptest.NewRequest("GET",
		fmt.Sprintf("/orders/verify?order_id=%s&tx_hash=sig-x", order.OrderUUID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// CompleteOrder transitions the row exactly once: concurrent/double
// verification of the same order cannot credit the balance twice.
func TestCompleteOrder_Atomic(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()

	order, err := env.repo.CreateOrder(ctx, "BuyerWallet777", "solana", 10, 9.99, "solana", 0.5, "MerchantAddr777")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	ok1, err := env.repo.CompleteOrder(ctx, order.OrderUUID, "sig-once")
	if err != nil || !ok1 {
		t.Fatalf("first completion must succeed: ok=%v err=%v", ok1, err)
	}
	ok2, err := env.repo.CompleteOrder(ctx, order.OrderUUID, "sig-once")
	if err != nil {
		t.Fatalf("second completion must not error: %v", err)
	}
	if ok2 {
		t.Fatal("second completion must report no transition")
	}
}
