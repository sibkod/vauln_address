package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Solana transaction verification for payment orders. The verifier fetches
// the transaction from an RPC node and proves that the order was actually
// paid: the tx succeeded on-chain, the order's payment address received at
// least the required amount of SOL, and the paying wallet matches the order.

// solanaTxFreshnessWindow is how old a payment transaction may be relative
// to the order creation. Older transactions are rejected so that an old
// transfer to the payment address cannot be replayed to unlock new orders.
const solanaTxFreshnessWindow = 2 * time.Hour

// paymentSlackRatio tolerates rounding in the SOL amount shown to the user
// (CreateOrder rounds to 4 decimal places) — 0.5% of the expected amount.
const paymentSlackRatio = 0.005

const lamportsPerSOL = 1_000_000_000

// VerificationError carries a machine-readable rejection code alongside the
// human-readable reason.
type VerificationError struct {
	Code string
	Msg  string
}

func (e *VerificationError) Error() string { return e.Msg }

// PaymentVerification is the outcome of a successful on-chain check.
type PaymentVerification struct {
	// PaidLamports is how much the payment address gained in this tx.
	PaidLamports int64
	// Sender is the fee payer of the transaction.
	Sender string
	// BlockTime is the on-chain timestamp of the transaction.
	BlockTime time.Time
}

// PaymentVerifier verifies Solana payment transactions against an order.
// The RPC endpoint and HTTP client are fields so tests can point the
// verifier at a mock server.
type PaymentVerifier struct {
	RPCURL     string
	HTTPClient *http.Client
}

// NewPaymentVerifier creates a verifier for the given RPC endpoint.
func NewPaymentVerifier(rpcURL string) *PaymentVerifier {
	return &PaymentVerifier{
		RPCURL:     rpcURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// solanaTx is the subset of getTransaction(jsonParsed) the verifier needs.
type solanaTx struct {
	slot      uint64
	blockTime time.Time
	err       any
	feePayer  string
	// owner (lowercased) -> lamports gained (post-pre), negative when spent
	balanceDiff map[string]int64
	// destination (lowercased) -> lamports sent via system transfer
	systemTransfers map[string]uint64
}

// VerifyPaymentTx proves that tx signature paid `expectedSOL` (or more)
// from `sender` to `recipient`. `orderCreatedAt` bounds the tx age. Any
// mismatch is a *VerificationError; transport/RPC problems are plain errors
// (callers should treat them as retryable, not as rejection).
func (v *PaymentVerifier) VerifyPaymentTx(ctx context.Context, signature, sender, recipient string, expectedSOL float64, orderCreatedAt time.Time) (*PaymentVerification, error) {
	tx, err := v.fetchTransaction(ctx, signature)
	if err != nil {
		return nil, err
	}

	if tx.err != nil {
		return nil, &VerificationError{Code: "TX_FAILED", Msg: "transaction failed on-chain"}
	}
	if !tx.blockTime.IsZero() {
		if tx.blockTime.Before(orderCreatedAt.Add(-solanaTxFreshnessWindow)) {
			return nil, &VerificationError{Code: "TX_TOO_OLD", Msg: "transaction is older than the order"}
		}
		if tx.blockTime.After(time.Now().Add(10 * time.Minute)) {
			return nil, &VerificationError{Code: "TX_IN_FUTURE", Msg: "transaction timestamp is in the future"}
		}
	}
	if sender != "" && tx.feePayer != "" && !strings.EqualFold(tx.feePayer, sender) {
		return nil, &VerificationError{Code: "WRONG_SENDER", Msg: "transaction was not sent from the order wallet"}
	}

	expectedLamports := int64(expectedSOL * lamportsPerSOL * (1 - paymentSlackRatio))
	paid := tx.paidTo(recipient)
	if paid < expectedLamports {
		return nil, &VerificationError{
			Code: "AMOUNT_MISMATCH",
			Msg:  fmt.Sprintf("payment address received %.6f SOL, expected at least %.4f SOL", float64(paid)/lamportsPerSOL, expectedSOL),
		}
	}

	return &PaymentVerification{
		PaidLamports: paid,
		Sender:       tx.feePayer,
		BlockTime:    tx.blockTime,
	}, nil
}

// paidTo reports how many lamports `recipient` gained in the transaction.
// Direct wallet-to-wallet payments raise the recipient's balance, so the
// balance diff is the primary source; when the recipient pays the fee (its
// net diff is negative) the parsed system-transfer instructions are used.
func (tx *solanaTx) paidTo(recipient string) int64 {
	key := strings.ToLower(recipient)
	if diff := tx.balanceDiff[key]; diff > 0 {
		return diff
	}
	if tx.balanceDiff[key] < 0 {
		return 0 // the "recipient" funded this tx — not a payment to it
	}
	if amt := tx.systemTransfers[key]; amt > 0 {
		return int64(amt)
	}
	return tx.balanceDiff[key]
}

// ==================== RPC plumbing ====================

func (v *PaymentVerifier) fetchTransaction(ctx context.Context, signature string) (*solanaTx, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getTransaction",
		"params": []interface{}{
			signature,
			map[string]interface{}{
				"encoding":                       "jsonParsed",
				"maxSupportedTransactionVersion": 0,
			},
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.RPCURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := v.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("RPC request failed: %w", err)
	}
	defer resp.Body.Close()

	var rpcResp struct {
		Result *struct {
			Slot      uint64 `json:"slot"`
			BlockTime *int64 `json:"blockTime"`
			Meta      *struct {
				Err               any      `json:"err"`
				PreBalances       []uint64 `json:"preBalances"`
				PostBalances      []uint64 `json:"postBalances"`
			} `json:"meta"`
			Transaction struct {
				Message struct {
					AccountKeys []struct {
						Pubkey string `json:"pubkey"`
					} `json:"accountKeys"`
					Instructions []json.RawMessage `json:"instructions"`
				} `json:"message"`
			} `json:"transaction"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("failed to parse RPC response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", rpcResp.Error.Message)
	}
	if rpcResp.Result == nil || rpcResp.Result.Meta == nil {
		return nil, fmt.Errorf("transaction not found: %s", signature)
	}

	res := rpcResp.Result
	tx := &solanaTx{
		slot:            res.Slot,
		err:             res.Meta.Err,
		balanceDiff:     map[string]int64{},
		systemTransfers: map[string]uint64{},
	}
	if res.BlockTime != nil {
		tx.blockTime = time.Unix(*res.BlockTime, 0).UTC()
	}
	keys := res.Transaction.Message.AccountKeys
	if len(keys) > 0 {
		tx.feePayer = keys[0].Pubkey
	}
	for i, key := range keys {
		if i >= len(res.Meta.PreBalances) || i >= len(res.Meta.PostBalances) {
			break
		}
		tx.balanceDiff[strings.ToLower(key.Pubkey)] += int64(res.Meta.PostBalances[i]) - int64(res.Meta.PreBalances[i])
	}
	for _, raw := range res.Transaction.Message.Instructions {
		if dest, amt, ok := parseSystemTransfer(raw); ok {
			tx.systemTransfers[strings.ToLower(dest)] += amt
		}
	}
	return tx, nil
}

// parseSystemTransfer extracts the destination and lamport amount from a
// system-program transfer instruction in jsonParsed form
// ({program: "system", parsed: {type: "transfer", info: {...}}}). Raw
// (non-parsed) instructions are skipped: the account balance diff already
// covers direct wallet-to-wallet payments, this fallback exists for cases
// where the payment address itself funds the transaction (e.g. a merchant
// account paying the network fee of the customer's transfer).
func parseSystemTransfer(raw json.RawMessage) (string, uint64, bool) {
	var ix struct {
		Program string `json:"program"`
		Parsed  *struct {
			Type string `json:"type"`
			Info struct {
				Destination string `json:"destination"`
				Lamports    uint64 `json:"lamports"`
			} `json:"info"`
		} `json:"parsed"`
	}
	if err := json.Unmarshal(raw, &ix); err != nil {
		return "", 0, false
	}
	if ix.Parsed == nil || ix.Program != "system" || ix.Parsed.Type != "transfer" || ix.Parsed.Info.Destination == "" {
		return "", 0, false
	}
	return ix.Parsed.Info.Destination, ix.Parsed.Info.Lamports, true
}
