package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// rpcResponse builds a getTransaction result JSON for a tx where `feePayer`
// paid `paidLamports` to `recipient` at `blockTime` (0 = omit). `metaErr`
// simulates a failed on-chain transaction.
func rpcResponse(t *testing.T, feePayer, recipient string, paidLamports uint64, blockTime int64, metaErr any) map[string]interface{} {
	t.Helper()
	fee := uint64(5000)
	pre := []uint64{10_000_000_000, 0}
	post := []uint64{10_000_000_000 - paidLamports - fee, paidLamports}
	keys := []map[string]interface{}{
		{"pubkey": feePayer, "signer": true},
		{"pubkey": recipient, "signer": false},
	}
	result := map[string]interface{}{
		"slot":      12345,
		"blockTime": blockTime,
		"meta": map[string]interface{}{
			"err":          metaErr,
			"fee":          fee,
			"preBalances":  pre,
			"postBalances": post,
		},
		"transaction": map[string]interface{}{
			"message": map[string]interface{}{
				"accountKeys": keys,
				"instructions": []interface{}{
					map[string]interface{}{
						"program": "system",
						"parsed": map[string]interface{}{
							"type": "transfer",
							"info": map[string]interface{}{
								"source":      feePayer,
								"destination": recipient,
								"lamports":    paidLamports,
							},
						},
					},
				},
			},
		},
	}
	if blockTime == 0 {
		delete(result, "blockTime")
	}
	return map[string]interface{}{"jsonrpc": "2.0", "id": 1, "result": result}
}

func mockRPC(t *testing.T, payload map[string]interface{}) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestVerifyPaymentTx_Confirmed(t *testing.T) {
	now := time.Now()
	srv := mockRPC(t, rpcResponse(t, "SenderWallet", "PaymentAddr", 500_000_000, now.Unix(), nil))
	v := NewPaymentVerifier(srv.URL)

	res, err := v.VerifyPaymentTx(context.Background(), "sig-ok", "SenderWallet", "PaymentAddr", 0.5, now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if res.PaidLamports != 500_000_000 {
		t.Fatalf("expected 0.5 SOL, got %d lamports", res.PaidLamports)
	}
	if res.Sender != "SenderWallet" {
		t.Fatalf("unexpected sender %q", res.Sender)
	}
}

func TestVerifyPaymentTx_FailedOnChain(t *testing.T) {
	now := time.Now()
	srv := mockRPC(t, rpcResponse(t, "SenderWallet", "PaymentAddr", 500_000_000, now.Unix(), map[string]interface{}{"InstructionError": []interface{}{0, "Custom"}}))
	v := NewPaymentVerifier(srv.URL)

	_, err := v.VerifyPaymentTx(context.Background(), "sig-fail", "SenderWallet", "PaymentAddr", 0.5, now.Add(-time.Minute))
	var verr *VerificationError
	if !errors.As(err, &verr) || verr.Code != "TX_FAILED" {
		t.Fatalf("expected TX_FAILED, got %v", err)
	}
}

func TestVerifyPaymentTx_WrongRecipient(t *testing.T) {
	now := time.Now()
	// tx pays SomebodyElse, not the order's payment address
	srv := mockRPC(t, rpcResponse(t, "SenderWallet", "SomebodyElse", 500_000_000, now.Unix(), nil))
	v := NewPaymentVerifier(srv.URL)

	_, err := v.VerifyPaymentTx(context.Background(), "sig-wrong", "SenderWallet", "PaymentAddr", 0.5, now.Add(-time.Minute))
	var verr *VerificationError
	if !errors.As(err, &verr) || verr.Code != "AMOUNT_MISMATCH" {
		t.Fatalf("expected AMOUNT_MISMATCH, got %v", err)
	}
}

func TestVerifyPaymentTx_Underpaid(t *testing.T) {
	now := time.Now()
	srv := mockRPC(t, rpcResponse(t, "SenderWallet", "PaymentAddr", 100_000_000, now.Unix(), nil))
	v := NewPaymentVerifier(srv.URL)

	_, err := v.VerifyPaymentTx(context.Background(), "sig-low", "SenderWallet", "PaymentAddr", 0.5, now.Add(-time.Minute))
	var verr *VerificationError
	if !errors.As(err, &verr) || verr.Code != "AMOUNT_MISMATCH" {
		t.Fatalf("expected AMOUNT_MISMATCH, got %v", err)
	}
}

func TestVerifyPaymentTx_WrongSender(t *testing.T) {
	now := time.Now()
	srv := mockRPC(t, rpcResponse(t, "SomebodyElse", "PaymentAddr", 500_000_000, now.Unix(), nil))
	v := NewPaymentVerifier(srv.URL)

	_, err := v.VerifyPaymentTx(context.Background(), "sig-sender", "SenderWallet", "PaymentAddr", 0.5, now.Add(-time.Minute))
	var verr *VerificationError
	if !errors.As(err, &verr) || verr.Code != "WRONG_SENDER" {
		t.Fatalf("expected WRONG_SENDER, got %v", err)
	}
}

func TestVerifyPaymentTx_TooOld(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	srv := mockRPC(t, rpcResponse(t, "SenderWallet", "PaymentAddr", 500_000_000, old.Unix(), nil))
	v := NewPaymentVerifier(srv.URL)

	_, err := v.VerifyPaymentTx(context.Background(), "sig-old", "SenderWallet", "PaymentAddr", 0.5, now.Add(-time.Minute))
	var verr *VerificationError
	if !errors.As(err, &verr) || verr.Code != "TX_TOO_OLD" {
		t.Fatalf("expected TX_TOO_OLD, got %v", err)
	}
}

func TestVerifyPaymentTx_NotFoundIsRetryable(t *testing.T) {
	srv := mockRPC(t, map[string]interface{}{"jsonrpc": "2.0", "id": 1, "result": nil})
	v := NewPaymentVerifier(srv.URL)

	_, err := v.VerifyPaymentTx(context.Background(), "sig-missing", "SenderWallet", "PaymentAddr", 0.5, time.Now())
	var verr *VerificationError
	if errors.As(err, &verr) {
		t.Fatalf("not-found must be a retryable error, not a rejection: %v", verr)
	}
	if err == nil {
		t.Fatal("expected an error for a missing transaction")
	}
}

// Self-transfer: the "recipient" funds the tx (negative balance diff) and
// the transfer instruction moves nothing — no payment happened.
func TestVerifyPaymentTx_RecipientPaysFee(t *testing.T) {
	now := time.Now()
	resp := rpcResponse(t, "PaymentAddr", "PaymentAddr", 0, now.Unix(), nil)
	result := resp["result"].(map[string]interface{})
	meta := result["meta"].(map[string]interface{})
	meta["preBalances"] = []uint64{10_000_000_000, 0}
	meta["postBalances"] = []uint64{9_999_995_000, 0}
	srv := mockRPC(t, resp)
	v := NewPaymentVerifier(srv.URL)

	_, err := v.VerifyPaymentTx(context.Background(), "sig-self", "SenderWallet", "PaymentAddr", 0.5, now.Add(-time.Minute))
	var verr *VerificationError
	if !errors.As(err, &verr) {
		t.Fatalf("self-transfer must be rejected, got %v", err)
	}
}
