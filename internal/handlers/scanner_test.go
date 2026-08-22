package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"vauln-address/internal/middleware"
	"vauln-address/internal/models"
	"vauln-address/internal/repository"
)

const scanTestAdminKey = "test-admin-key"

func setupScannerRouter(env *reportTestEnv) *gin.Engine {
	env.handler.serverCfg.AdminAPIKey = scanTestAdminKey

	router := gin.New()
	admin := router.Group("/api/admin")
	admin.Use(middleware.AdminMiddleware(scanTestAdminKey))
	admin.POST("/scanner/findings", env.handler.IngestScanFinding)
	router.GET("/api/monitor/findings", env.handler.GetMonitorFindings)
	router.GET("/api/monitor/stats", env.handler.GetMonitorStats)
	router.GET("/api/captcha", env.handler.GetCaptcha)
	router.POST("/api/drainer-reports", env.handler.SubmitDrainerReport)
	router.GET("/api/report", env.handler.GetReport)
	return router
}

func postFinding(t *testing.T, router *gin.Engine, adminKey string, body models.ScanFindingRequest) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal finding: %v", err)
	}
	req := httptest.NewRequest("POST", "/api/admin/scanner/findings", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if adminKey != "" {
		req.Header.Set("X-Admin-Key", adminKey)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// Valid 32-byte base58 addresses used across scanner tests — the ingest
// handler rejects anything that does not decode to a Solana public key.
const (
	scanAddrVictim  = "9ML9o4nY6i54JPUcA8oDKFWQAsFR4ht3hXf6WdtmCpvw"
	scanAddrHacker  = "4QFiKg8ejx5LfqqLNbnKsiCnbEgRtgakBF6abMrkquKW"
	scanAddrSender1 = "27Jc8szpEz1PiDTsF1QTs4PLLD8izBEWkfYPxA9drq7Z"
	scanAddrSender2 = "56qGwVvKnGnCHid5cn1hVNHPJ2ny6iAwFd9825y2WkSk"
	scanAddrSender3 = "9cgS3VZhoJgaRpbfbFzAUrHxcXzhGZB1uzBXJJiZpwa"
	scanAddrSender4 = "HjPZYpzbN6UspYq1jtF51A5LXZBpwLGUqV2KVrRuRjYo"
)

func sampleFinding(sig, victim, hacker string) models.ScanFindingRequest {
	return models.ScanFindingRequest{
		Signature:     sig,
		Slot:          123456,
		Verdict:       models.ScanVerdictDrainer,
		Indicators:    []string{"P2_FULL_BALANCE_SWEEP", "P3_UNKNOWN_PROGRAM"},
		VictimAddress: victim,
		HackerAddress: hacker,
		AmountSOL:     1.5,
		Programs:      []string{"EtrnLzgbS7nMMy5fbD42kXiUzGg8XQzJ972Xtk1cjWih"},
		Source:        "watch",
	}
}

// TestIngestScanFinding_AdminKeyRequired rejects unauthenticated ingest.
func TestIngestScanFinding_AdminKeyRequired(t *testing.T) {
	env := setupReportTest(t)
	router := setupScannerRouter(env)

	w := postFinding(t, router, "", sampleFinding("sig1", "victimA", "hackerA"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without admin key, got %d", w.Code)
	}

	w = postFinding(t, router, "wrong-key", sampleFinding("sig1", "victimA", "hackerA"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong admin key, got %d", w.Code)
	}
}

// TestIngestScanFinding_RegistersWallets verifies that a DRAINER finding is
// stored and both parties land in the wallets table: victim as "drained",
// hacker as "hacker".
func TestIngestScanFinding_RegistersWallets(t *testing.T) {
	env := setupReportTest(t)
	router := setupScannerRouter(env)
	ctx := context.Background()

	victim := scanAddrVictim
	hacker := scanAddrHacker

	w := postFinding(t, router, scanTestAdminKey, sampleFinding("sig-drain-1", victim, hacker))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		ID          int64 `json:"id"`
		Inserted    bool  `json:"inserted"`
		VictimAdded bool  `json:"victim_added"`
		HackerAdded bool  `json:"hacker_added"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Inserted || !resp.VictimAdded || !resp.HackerAdded || resp.ID == 0 {
		t.Errorf("unexpected ingest response: %+v", resp)
	}

	victimWallet, err := env.repo.GetWallet(ctx, victim, "solana")
	if err != nil || victimWallet == nil {
		t.Fatalf("victim wallet not registered: %v", err)
	}
	if victimWallet.Status != models.StatusDrained {
		t.Errorf("expected victim status drained, got %q", victimWallet.Status)
	}

	hackerWallet, err := env.repo.GetWallet(ctx, hacker, "solana")
	if err != nil || hackerWallet == nil {
		t.Fatalf("hacker wallet not registered: %v", err)
	}
	if hackerWallet.Status != models.StatusHacker {
		t.Errorf("expected hacker status hacker, got %q", hackerWallet.Status)
	}

	// duplicate signature is a no-op
	w = postFinding(t, router, scanTestAdminKey, sampleFinding("sig-drain-1", victim, hacker))
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode dup response: %v", err)
	}
	if resp.Inserted {
		t.Error("duplicate signature must not be inserted twice")
	}
}

// TestIngestScanFinding_MarksAssociated verifies that wallets that sent funds
// to the drainer operator get the associated_hacker flag: unknown ones are
// registered as "unknown", existing statuses are never overridden.
func TestIngestScanFinding_MarksAssociated(t *testing.T) {
	env := setupReportTest(t)
	router := setupScannerRouter(env)
	ctx := context.Background()

	// An already-known wallet must keep its status, only gain the flag.
	if _, err := env.repo.CreateWallet(ctx, scanAddrSender2, "solana", models.StatusPhishing, "", ""); err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}

	payload := sampleFinding("sig-drain-assoc", scanAddrVictim, scanAddrHacker)
	payload.ExposedAddresses = []string{scanAddrSender1, scanAddrSender2, scanAddrHacker, ""}

	w := postFinding(t, router, scanTestAdminKey, payload)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Associated int `json:"associated"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Associated != 2 {
		t.Errorf("expected 2 associated addresses (dupes/self skipped), got %d", resp.Associated)
	}

	fresh, err := env.repo.GetWallet(ctx, scanAddrSender1, "solana")
	if err != nil || fresh == nil {
		t.Fatalf("senderNew not registered: %v", err)
	}
	if fresh.Status != models.StatusUnknown {
		t.Errorf("expected senderNew status unknown, got %q", fresh.Status)
	}
	if !fresh.AssociatedHacker || fresh.AssociatedReason == "" {
		t.Errorf("senderNew must carry the association flag and reason: %+v", fresh)
	}

	known, err := env.repo.GetWallet(ctx, scanAddrSender2, "solana")
	if err != nil || known == nil {
		t.Fatalf("senderKnown lookup failed: %v", err)
	}
	if known.Status != models.StatusPhishing {
		t.Errorf("existing status must not be overridden, got %q", known.Status)
	}
	if !known.AssociatedHacker {
		t.Error("senderKnown must be flagged as associated")
	}

	// SUSPICIOUS verdicts never mark associations.
	payload2 := sampleFinding("sig-susp-assoc", scanAddrVictim, scanAddrSender4)
	payload2.Verdict = models.ScanVerdictSuspicious
	payload2.ExposedAddresses = []string{scanAddrSender3}
	w = postFinding(t, router, scanTestAdminKey, payload2)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	susp, err := env.repo.GetWallet(ctx, scanAddrSender3, "solana")
	if err != nil {
		t.Fatalf("senderSusp lookup failed: %v", err)
	}
	if susp != nil && susp.AssociatedHacker {
		t.Error("suspicious findings must not mark associations")
	}
}

// TestIngestScanFinding_FiltersInvalidAddresses verifies that placeholder
// strings which fail strict base58-32B validation are dropped from the
// finding before wallet registration: the finding itself is stored, but the
// bogus counterparty never becomes a wallet.
func TestIngestScanFinding_FiltersInvalidAddresses(t *testing.T) {
	env := setupReportTest(t)
	router := setupScannerRouter(env)
	ctx := context.Background()

	payload := sampleFinding("sig-invalid-addr", "VictimWallet1111111111111111111111111111", scanAddrHacker)
	payload.ExposedAddresses = []string{scanAddrSender1, "X5eq6Ho3abcdefghijklmno1234567890123456789AB"}

	w := postFinding(t, router, scanTestAdminKey, payload)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		VictimAdded bool `json:"victim_added"`
		HackerAdded bool `json:"hacker_added"`
		Associated  int  `json:"associated"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.VictimAdded {
		t.Error("invalid victim address must not be registered as a wallet")
	}
	if !resp.HackerAdded {
		t.Error("valid hacker address must be registered")
	}
	if resp.Associated != 1 {
		t.Errorf("expected only 1 valid associated address, got %d", resp.Associated)
	}

	if w1, _ := env.repo.GetWallet(ctx, "VictimWallet1111111111111111111111111111", "solana"); w1 != nil {
		t.Error("invalid victim placeholder must not appear in wallets")
	}
	if w2, _ := env.repo.GetWallet(ctx, "X5eq6Ho3abcdefghijklmno1234567890123456789AB", "solana"); w2 != nil {
		t.Error("invalid exposed placeholder must not appear in wallets")
	}
}

// TestMonitorEndpoints covers the live feed: latest findings, incremental
// after_id polling and aggregate stats.
func TestMonitorEndpoints(t *testing.T) {
	env := setupReportTest(t)
	router := setupScannerRouter(env)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		_, _, err := env.repo.InsertScanFinding(ctx, models.ScanFindingRequest{
			Signature:     fmt.Sprintf("sig-%d", i),
			Verdict:       models.ScanVerdictDrainer,
			Indicators:    []string{"P2_FULL_BALANCE_SWEEP"},
			VictimAddress: fmt.Sprintf("victim-%d", i),
			HackerAddress: "hacker-x",
			AmountSOL:     float64(i),
			Source:        "watch",
		})
		if err != nil {
			t.Fatalf("InsertScanFinding: %v", err)
		}
	}
	_, _, err := env.repo.InsertScanFinding(ctx, models.ScanFindingRequest{
		Signature: "sig-susp", Verdict: models.ScanVerdictSuspicious,
		Indicators: []string{"P4_CONTROL_ACCOUNT"}, Source: "watch",
	})
	if err != nil {
		t.Fatalf("InsertScanFinding suspicious: %v", err)
	}

	// latest feed, newest first
	req := httptest.NewRequest("GET", "/api/monitor/findings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("findings: expected 200, got %d", w.Code)
	}
	var feed struct {
		Findings []models.ScanFinding `json:"findings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &feed); err != nil {
		t.Fatalf("decode findings: %v", err)
	}
	if len(feed.Findings) != 4 {
		t.Fatalf("expected 4 findings, got %d", len(feed.Findings))
	}
	if feed.Findings[0].ID < feed.Findings[1].ID {
		t.Error("latest feed must be newest-first")
	}

	// incremental polling returns only newer rows
	afterID := feed.Findings[0].ID
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/monitor/findings?after_id=%d", afterID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &feed); err != nil {
		t.Fatalf("decode incremental findings: %v", err)
	}
	if len(feed.Findings) != 0 {
		t.Errorf("expected 0 new findings after id %d, got %d", afterID, len(feed.Findings))
	}

	// the feed is hard-capped at the latest monitorFeedLimit rows:
	// older findings stay in the database but are never served
	for i := 4; i <= 12; i++ {
		_, _, err := env.repo.InsertScanFinding(ctx, models.ScanFindingRequest{
			Signature:  fmt.Sprintf("sig-%d", i),
			Verdict:    models.ScanVerdictDrainer,
			Indicators: []string{"P2_FULL_BALANCE_SWEEP"},
			Source:     "watch",
		})
		if err != nil {
			t.Fatalf("InsertScanFinding %d: %v", i, err)
		}
	}
	req = httptest.NewRequest("GET", "/api/monitor/findings", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &feed); err != nil {
		t.Fatalf("decode capped feed: %v", err)
	}
	if len(feed.Findings) != 10 {
		t.Errorf("feed must be capped at 10 findings, got %d", len(feed.Findings))
	}
	if feed.Findings[0].Signature != "sig-12" {
		t.Errorf("feed must start with the newest finding, got %q", feed.Findings[0].Signature)
	}
	if feed.Findings[len(feed.Findings)-1].Signature != "sig-susp" {
		t.Errorf("feed must end at the 10th newest finding, got %q", feed.Findings[len(feed.Findings)-1].Signature)
	}

	// stats
	req = httptest.NewRequest("GET", "/api/monitor/stats", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var stats models.ScanStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.TotalFindings != 13 || stats.DrainerCount != 12 || stats.SuspectCount != 1 {
		t.Errorf("unexpected stats: %+v", stats)
	}
	if stats.VictimCount != 3 || stats.HackerCount != 1 {
		t.Errorf("unexpected party counts: %+v", stats)
	}
	if stats.StolenSOL != 6 {
		t.Errorf("expected 6 stolen SOL, got %v", stats.StolenSOL)
	}
}

// TestSubmitDrainerReport_CaptchaRequired rejects a wrong captcha answer.
func TestSubmitDrainerReport_CaptchaRequired(t *testing.T) {
	env := setupReportTest(t)
	router := setupScannerRouter(env)

	body := `{"tx_signature":"5Kd4fakeSignaturefakeSignaturefakeSignaturefakeSign12","captcha_id":"nope","captcha_answer":"XXXXX"}`
	req := httptest.NewRequest("POST", "/api/drainer-reports", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for invalid captcha, got %d", w.Code)
	}
	resp := decodeError(t, w)
	if resp.Code != "CAPTCHA_INVALID" {
		t.Errorf("expected CAPTCHA_INVALID, got %q", resp.Code)
	}
}

// TestSubmitDrainerReport_Success covers the full captcha flow: GET a
// challenge, answer it correctly, the report is stored and (without a
// configured bot) marked as not telegram-sent.
func TestSubmitDrainerReport_Success(t *testing.T) {
	env := setupReportTest(t)
	router := setupScannerRouter(env)
	ctx := context.Background()

	req := httptest.NewRequest("GET", "/api/captcha", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("captcha: expected 200, got %d", w.Code)
	}
	var captcha models.CaptchaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &captcha); err != nil {
		t.Fatalf("decode captcha: %v", err)
	}
	if captcha.CaptchaID == "" || captcha.Image == "" {
		t.Fatalf("captcha response is incomplete: %+v", captcha)
	}

	answer, ok := env.handler.captcha.Answer(captcha.CaptchaID)
	if !ok {
		t.Fatal("captcha challenge not found in service")
	}

	payload, _ := json.Marshal(map[string]string{
		"tx_signature":   "5Kd4realSignatureThatIsLongEnoughToPassValidation0123456789",
		"chain":          "solana",
		"site_url":       "fake-airdrop.example",
		"description":    "signed a transaction on a phishing site, all SOL drained",
		"captcha_id":     captcha.CaptchaID,
		"captcha_answer": answer,
	})
	req = httptest.NewRequest("POST", "/api/drainer-reports", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.50:1234"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		ID           int64 `json:"id"`
		TelegramSent bool  `json:"telegram_sent"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode report response: %v", err)
	}
	if resp.ID == 0 {
		t.Error("expected report id in response")
	}
	if resp.TelegramSent {
		t.Error("telegram must be unsent when the bot is not configured")
	}

	// the site URL was normalized to include the scheme
	var site string
	if err := env.db.QueryRow(`SELECT site_url FROM drainer_reports WHERE id = ?`, resp.ID).Scan(&site); err != nil {
		t.Fatalf("read drainer_report: %v", err)
	}
	if site != "https://fake-airdrop.example" {
		t.Errorf("expected normalized site URL, got %q", site)
	}

	// a captcha is single-use: replaying the same answer must fail
	req = httptest.NewRequest("POST", "/api/drainer-reports", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 on captcha replay, got %d", w.Code)
	}

	_ = ctx
}

// TestReportEvidenceChain verifies the report contains the status evidence:
// the registry listing, the leak, and the scanner indicators with their txs.
func TestReportEvidenceChain(t *testing.T) {
	env := setupReportTest(t)
	router := setupScannerRouter(env)
	ctx := context.Background()

	env.seedHackedWallet(t, reportTestAddr, "evm")

	requester := repository.AnonymousRequesterPrefix + "203.0.113.9"
	if err := env.repo.RecordCheck(ctx, requester, reportTestAddr, "evm", "hacked"); err != nil {
		t.Fatalf("RecordCheck: %v", err)
	}

	_, _, err := env.repo.InsertScanFinding(ctx, models.ScanFindingRequest{
		Signature:     "evidence-tx-1",
		Verdict:       models.ScanVerdictDrainer,
		Indicators:    []string{"P2_FULL_BALANCE_SWEEP"},
		VictimAddress: reportTestAddr,
		HackerAddress: "HackerWallet999",
		AmountSOL:     0.75,
		Source:        "watch",
	})
	if err != nil {
		t.Fatalf("InsertScanFinding: %v", err)
	}

	w := doReportRequest(router, reportTestAddr, "evm", "203.0.113.9:4567")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	report := decodeReport(t, w)

	if len(report.Evidence) == 0 {
		t.Fatal("expected non-empty evidence chain")
	}

	var hasRegistry, hasLeak, hasScanner bool
	for _, e := range report.Evidence {
		switch e.Code {
		case "registry":
			hasRegistry = true
		case "key_leak":
			hasLeak = true
		case "P2_FULL_BALANCE_SWEEP":
			hasScanner = true
			if e.TxSignature != "evidence-tx-1" {
				t.Errorf("scanner evidence must carry the tx signature, got %q", e.TxSignature)
			}
			if e.Counterparty != "HackerWallet999" {
				t.Errorf("expected hacker as counterparty, got %q", e.Counterparty)
			}
			if e.AmountSOL != 0.75 {
				t.Errorf("expected amount 0.75, got %v", e.AmountSOL)
			}
		}
	}
	if !hasRegistry || !hasLeak || !hasScanner {
		t.Errorf("evidence chain incomplete (registry=%v leak=%v scanner=%v): %+v",
			hasRegistry, hasLeak, hasScanner, report.Evidence)
	}
}

// TestIngestScanFinding_ProgramNotRegisteredAsHacker: when the hacker address
// of a finding is one of the on-chain programs the transaction invoked, it is
// only a transit point for the stolen funds, so it must not land in the
// wallets table as a hacker.
func TestIngestScanFinding_ProgramNotRegisteredAsHacker(t *testing.T) {
	env := setupReportTest(t)
	router := setupScannerRouter(env)

	finding := sampleFinding("sig-prog-1", scanAddrVictim, scanAddrSender3)
	finding.Programs = []string{scanAddrSender3, "EtrnLzgbS7nMMy5fbD42kXiUzGg8XQzJ972Xtk1cjWih"}

	w := postFinding(t, router, scanTestAdminKey, finding)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Inserted    bool `json:"inserted"`
		VictimAdded bool `json:"victim_added"`
		HackerAdded bool `json:"hacker_added"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Inserted || !resp.VictimAdded {
		t.Errorf("unexpected ingest response: %+v", resp)
	}
	if resp.HackerAdded {
		t.Error("program address must not be registered as a hacker wallet")
	}

	wlt, err := env.repo.GetWallet(context.Background(), scanAddrSender3, "solana")
	if err != nil {
		t.Fatalf("GetWallet: %v", err)
	}
	if wlt != nil {
		t.Errorf("program %s must stay out of the wallets table, got status %q", scanAddrSender3, wlt.Status)
	}
}

// TestIngestScanFinding_ReviewForwarding: a finding that matched the drainer
// pattern on a program that is not in the watchlist (no P5 indicator) is
// flagged for analyst review; known-program findings and duplicate
// re-submissions are not.
func TestIngestScanFinding_ReviewForwarding(t *testing.T) {
	env := setupReportTest(t)
	router := setupScannerRouter(env)

	decode := func(w *httptest.ResponseRecorder) (bool, bool) {
		var resp struct {
			Inserted    bool `json:"inserted"`
			NeedsReview bool `json:"needs_review"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return resp.Inserted, resp.NeedsReview
	}

	// pattern-only finding (unknown program) -> review
	w := postFinding(t, router, scanTestAdminKey, sampleFinding("sig-review-1", scanAddrVictim, scanAddrHacker))
	inserted, review := decode(w)
	if !inserted || !review {
		t.Errorf("pattern finding without P5 must need review, inserted=%v review=%v", inserted, review)
	}

	// duplicate re-submission -> no review, no second telegram message
	w = postFinding(t, router, scanTestAdminKey, sampleFinding("sig-review-1", scanAddrVictim, scanAddrHacker))
	inserted, review = decode(w)
	if inserted || review {
		t.Errorf("duplicate finding must not need review, inserted=%v review=%v", inserted, review)
	}

	// finding that hit a known drainer program (P5) -> no review
	known := sampleFinding("sig-review-2", scanAddrSender1, scanAddrHacker)
	known.Indicators = append(known.Indicators, "P5_KNOWN_DRAINER_PROGRAM")
	w = postFinding(t, router, scanTestAdminKey, known)
	inserted, review = decode(w)
	if !inserted || review {
		t.Errorf("P5 finding must not need review, inserted=%v review=%v", inserted, review)
	}
}
