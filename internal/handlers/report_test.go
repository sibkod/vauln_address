package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"

	"vauln-address/internal/config"
	"vauln-address/internal/middleware"
	"vauln-address/internal/models"
	"vauln-address/internal/repository"
)

const reportTestAddr = "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1"

// reportTestEnv runs the real handler against a real SQLite database.
// db is a second connection used to seed rows the repository has no
// methods for (leaked_keys) and to backdate check_history timestamps.
type reportTestEnv struct {
	repo    *repository.Repository
	handler *Handler
	db      *sql.DB
}

func setupReportTest(t *testing.T) *reportTestEnv {
	t.Helper()

	dbPath := t.TempDir() + "/report_test.db"
	cfg := &config.Config{
		DBType:         config.DBTypeSQLite,
		SQLitePath:     dbPath,
		FreeCheckLimit: 3,
	}

	repo, err := repository.New(cfg)
	if err != nil {
		t.Fatalf("repository.New: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	raw, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	t.Cleanup(func() { raw.Close() })

	// leaked_keys is created by migration 003, not by runtime InitSchema
	_, err = raw.Exec(`CREATE TABLE IF NOT EXISTS leaked_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		wallet_id BIGINT NOT NULL,
		address VARCHAR(200) NOT NULL,
		chain VARCHAR(20) NOT NULL,
		key_type VARCHAR(20) NOT NULL,
		key_value TEXT NOT NULL,
		source VARCHAR(100),
		discovered_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create leaked_keys: %v", err)
	}

	return &reportTestEnv{
		repo:    repo,
		handler: New(repo, cfg, nil, nil),
		db:      raw,
	}
}

// seedHackedWallet creates the wallet row plus one leaked_keys entry.
func (e *reportTestEnv) seedHackedWallet(t *testing.T, address, chain string) {
	t.Helper()
	ctx := context.Background()

	walletID, err := e.repo.CreateWallet(ctx, address, chain, models.StatusHacked, "leaked private key", "github")
	if err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}

	_, err = e.db.Exec(
		`INSERT INTO leaked_keys (wallet_id, address, chain, key_type, key_value, source)
		 VALUES (?, ?, ?, 'private_key', '[ENCRYPTED]', 'github_leak')`,
		walletID, address, chain,
	)
	if err != nil {
		t.Fatalf("insert leaked_keys: %v", err)
	}
}

// backdateChecks moves the requester's check history rows into the past.
func (e *reportTestEnv) backdateChecks(t *testing.T, requester string, hours int) {
	t.Helper()
	ts := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := e.db.Exec(`UPDATE check_history SET created_at = ? WHERE wallet_address = ?`, ts, requester); err != nil {
		t.Fatalf("backdate check_history: %v", err)
	}
}

func doReportRequest(router *gin.Engine, address, chain, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/report?address=%s&chain=%s", address, chain), nil)
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeReport(t *testing.T, w *httptest.ResponseRecorder) models.ReportResponse {
	t.Helper()
	var resp models.ReportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode report response: %v (body: %s)", err, w.Body.String())
	}
	return resp
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) models.ErrorResponse {
	t.Helper()
	var resp models.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v (body: %s)", err, w.Body.String())
	}
	return resp
}

// TestGetReport_AnonymousFlow covers the happy path: hacked wallet with a
// leak, anonymous requester, full report with reason, details and tree.
func TestGetReport_AnonymousFlow(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()

	env.seedHackedWallet(t, reportTestAddr, "evm")

	requester := repository.AnonymousRequesterPrefix + "203.0.113.9"
	if err := env.repo.RecordCheck(ctx, requester, reportTestAddr, "evm", "hacked"); err != nil {
		t.Fatalf("RecordCheck: %v", err)
	}

	router := gin.New()
	router.GET("/api/report", env.handler.GetReport)

	w := doReportRequest(router, reportTestAddr, "evm", "203.0.113.9:4567")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	report := decodeReport(t, w)

	if !report.Found || report.Address != reportTestAddr || report.Chain != "evm" {
		t.Errorf("unexpected identity fields: %+v", report)
	}
	if report.Status != string(models.StatusHacked) {
		t.Errorf("expected status hacked, got %q", report.Status)
	}
	if report.Reason != "leaked private key" || report.Source != "github" {
		t.Errorf("expected reason/source from wallets row, got %q / %q", report.Reason, report.Source)
	}

	// hacked + leaked private key => details must say the key is public
	if !bytes.Contains([]byte(report.Details), []byte("private key")) ||
		!bytes.Contains([]byte(report.Details), []byte("publicly available")) {
		t.Errorf("details should explain that the private key is public, got %q", report.Details)
	}

	if len(report.Leaks) != 1 {
		t.Fatalf("expected 1 leak, got %d", len(report.Leaks))
	}
	leak := report.Leaks[0]
	if leak.KeyType != "private_key" || leak.Source != "github_leak" {
		t.Errorf("unexpected leak: %+v", leak)
	}
	if leak.DiscoveredAt.IsZero() {
		t.Error("leak discovered_at should be set")
	}

	// anonymous reports expire 24h after the check
	if report.ExpiresAt == nil {
		t.Fatal("anonymous report must carry expires_at")
	}
	untilExpiry := time.Until(*report.ExpiresAt)
	if untilExpiry < 23*time.Hour || untilExpiry > 25*time.Hour {
		t.Errorf("expires_at should be ~24h ahead, got %v from now", untilExpiry)
	}

	// transaction tree: built from indexed scanner findings - with none
	// the tree is just the root with zero real transactions
	root := report.Transactions
	if root == nil {
		t.Fatal("report must include the transaction tree")
	}
	if root.Address != reportTestAddr || root.Status != string(models.StatusHacked) {
		t.Errorf("unexpected tree root: %+v", root)
	}
	if root.Currency != "ETH" {
		t.Errorf("expected ETH currency for evm, got %q", root.Currency)
	}
	if root.TxCount != 0 || len(root.Children) != 0 {
		t.Errorf("without indexed findings the tree must be empty, got %d tx / %d children",
			root.TxCount, len(root.Children))
	}

	// the tree must be deterministic across requests
	w2 := doReportRequest(router, reportTestAddr, "evm", "203.0.113.9:4567")
	report2 := decodeReport(t, w2)
	tree1, _ := json.Marshal(report.Transactions)
	tree2, _ := json.Marshal(report2.Transactions)
	if !bytes.Equal(tree1, tree2) {
		t.Error("transaction tree must be identical across requests")
	}
}

// TestGetReport_AccessRules verifies who may open the report and for how long.
func TestGetReport_AccessRules(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()
	env.seedHackedWallet(t, reportTestAddr, "evm")

	router := gin.New()
	router.GET("/api/report", env.handler.GetReport)

	t.Run("not checked before", func(t *testing.T) {
		w := doReportRequest(router, reportTestAddr, "evm", "203.0.113.10:1")
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
		if code := decodeError(t, w).Code; code != "REPORT_NOT_AVAILABLE" {
			t.Errorf("expected REPORT_NOT_AVAILABLE, got %q", code)
		}
	})

	t.Run("anonymous report expires after 24h", func(t *testing.T) {
		requester := repository.AnonymousRequesterPrefix + "203.0.113.11"
		if err := env.repo.RecordCheck(ctx, requester, reportTestAddr, "evm", "hacked"); err != nil {
			t.Fatalf("RecordCheck: %v", err)
		}
		env.backdateChecks(t, requester, 25)

		w := doReportRequest(router, reportTestAddr, "evm", "203.0.113.11:1")
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
		if code := decodeError(t, w).Code; code != "REPORT_EXPIRED" {
			t.Errorf("expected REPORT_EXPIRED, got %q", code)
		}
	})

	t.Run("authenticated reports never expire", func(t *testing.T) {
		userWallet := "0xAAA35Cc6634C0532925a3b844Bc9e7595f5B2a1"
		if err := env.repo.RecordCheck(ctx, userWallet, reportTestAddr, "evm", "hacked"); err != nil {
			t.Fatalf("RecordCheck: %v", err)
		}
		env.backdateChecks(t, userWallet, 25*24) // 25 days

		authRouter := gin.New()
		authRouter.Use(func(c *gin.Context) { c.Set("userAddress", userWallet) })
		authRouter.GET("/api/report", env.handler.GetReport)

		w := doReportRequest(authRouter, reportTestAddr, "evm", "203.0.113.12:1")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if report := decodeReport(t, w); report.ExpiresAt != nil {
			t.Error("authenticated report must not carry expires_at")
		}
	})

	t.Run("checked but not found in database", func(t *testing.T) {
		unknown := "0xde0B295669a9FD93d5F28D9Ec85E40f4cb697BAe"
		requester := repository.AnonymousRequesterPrefix + "203.0.113.13"
		if err := env.repo.RecordCheck(ctx, requester, unknown, "evm", "safe"); err != nil {
			t.Fatalf("RecordCheck: %v", err)
		}

		w := doReportRequest(router, unknown, "evm", "203.0.113.13:1")
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
		if code := decodeError(t, w).Code; code != "NOT_FOUND" {
			t.Errorf("expected NOT_FOUND, got %q", code)
		}
	})
}

func TestGetReport_InvalidParams(t *testing.T) {
	env := setupReportTest(t)
	router := gin.New()
	router.GET("/api/report", env.handler.GetReport)

	tests := []struct {
		name         string
		query        string
		expectedCode string
	}{
		{"missing params", "", "INVALID_REQUEST"},
		{"missing chain", "?address=" + reportTestAddr, "INVALID_REQUEST"},
		{"unsupported chain", "?address=" + reportTestAddr + "&chain=doge", "INVALID_CHAIN"},
		{"invalid address", "?address=0x123&chain=evm", "INVALID_ADDRESS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/report"+tt.query, nil)
			req.RemoteAddr = "203.0.113.20:1"
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			if code := decodeError(t, w).Code; code != tt.expectedCode {
				t.Errorf("expected %s, got %q", tt.expectedCode, code)
			}
		})
	}
}

// TestGetReport_TreeFromScanFindings builds the transaction tree from real
// scan_findings: counterparties are aggregated per address, their statuses
// resolved against the wallets table, amounts and tx counts summed.
func TestGetReport_TreeFromScanFindings(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()
	env.seedHackedWallet(t, reportTestAddr, "evm")

	const hackerAddr = scanAddrHacker
	const otherVictim = scanAddrVictim
	if _, err := env.repo.CreateWallet(ctx, hackerAddr, "solana", models.StatusHacker, "", ""); err != nil {
		t.Fatalf("CreateWallet hacker: %v", err)
	}
	env.seedFinding(t, "sig-1", reportTestAddr, hackerAddr, 1.5)
	env.seedFinding(t, "sig-2", reportTestAddr, hackerAddr, 0.5)
	env.seedFinding(t, "sig-3", otherVictim, hackerAddr, 2.0)

	requester := repository.AnonymousRequesterPrefix + "203.0.113.15"
	if err := env.repo.RecordCheck(ctx, requester, reportTestAddr, "evm", "hacked"); err != nil {
		t.Fatalf("RecordCheck: %v", err)
	}

	router := gin.New()
	router.GET("/api/report", env.handler.GetReport)

	w := doReportRequest(router, reportTestAddr, "evm", "203.0.113.15:1")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	report := decodeReport(t, w)

	root := report.Transactions
	if root == nil {
		t.Fatal("report must include the transaction tree")
	}
	if root.TxCount != 2 || root.Amount != 2.0 {
		t.Errorf("root must aggregate its 2 indexed findings / 2.0, got %d / %f", root.TxCount, root.Amount)
	}
	if len(root.Children) != 1 {
		t.Fatalf("root must have the single counterparty as child, got %d", len(root.Children))
	}

	hackerNode := root.Children[0]
	if hackerNode.Address != hackerAddr {
		t.Errorf("expected child %s, got %s", hackerAddr, hackerNode.Address)
	}
	if hackerNode.Status != string(models.StatusHacker) {
		t.Errorf("counterparty in DB must show its DB status hacker, got %q", hackerNode.Status)
	}
	if hackerNode.TxCount != 3 || hackerNode.Amount != 4.0 {
		t.Errorf("hacker node shows its own totals: 3 tx / 4.0 expected, got %d / %f", hackerNode.TxCount, hackerNode.Amount)
	}

	// the tree stops at the hacker wallet: where the funds went afterwards
	// is out of scope for this report
	if len(hackerNode.Children) != 0 {
		t.Fatalf("hacker node must not be expanded, got %d children", len(hackerNode.Children))
	}

	// the tree must be deterministic across requests
	w2 := doReportRequest(router, reportTestAddr, "evm", "203.0.113.15:1")
	report2 := decodeReport(t, w2)
	tree1, _ := json.Marshal(report.Transactions)
	tree2, _ := json.Marshal(report2.Transactions)
	if !bytes.Equal(tree1, tree2) {
		t.Error("transaction tree must be identical across requests")
	}
}

// seedFinding inserts one scanner finding with a unique signature.
func (e *reportTestEnv) seedFinding(t *testing.T, signature, victim, hacker string, amount float64) {
	t.Helper()
	_, inserted, err := e.repo.InsertScanFinding(context.Background(), models.ScanFindingRequest{
		Chain:         "solana",
		Signature:     signature,
		Slot:          1,
		Verdict:       models.ScanVerdictDrainer,
		Indicators:    []string{"P2_FULL_BALANCE_SWEEP"},
		VictimAddress: victim,
		HackerAddress: hacker,
		AmountSOL:     amount,
		Source:        "test",
	})
	if err != nil || !inserted {
		t.Fatalf("InsertScanFinding %s: inserted=%v err=%v", signature, inserted, err)
	}
}

// TestCheckWallet_RecordsAnonymousIP verifies that an anonymous check is
// stored as "ip:<ip>" in check_history and immediately unlocks the report.
func TestCheckWallet_RecordsAnonymousIP(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()
	env.seedHackedWallet(t, reportTestAddr, "evm")

	router := gin.New()
	router.POST("/api/check", env.handler.CheckWallet)
	router.GET("/api/report", env.handler.GetReport)

	body, _ := json.Marshal(models.CheckRequest{Address: reportTestAddr, Chain: "evm"})
	req := httptest.NewRequest("POST", "/api/check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.7:8080"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("check failed: %d: %s", w.Code, w.Body.String())
	}

	// RecordCheck runs in a goroutine - wait until the row appears
	requester := repository.AnonymousRequesterPrefix + "198.51.100.7"
	deadline := time.Now().Add(3 * time.Second)
	for {
		last, err := env.repo.GetLastReportAccess(ctx, requester, reportTestAddr, "evm")
		if err != nil {
			t.Fatalf("GetLastReportAccess: %v", err)
		}
		if last != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("anonymous check was not recorded as ip:<ip>")
		}
		time.Sleep(20 * time.Millisecond)
	}

	w = doReportRequest(router, reportTestAddr, "evm", "198.51.100.7:9999")
	if w.Code != http.StatusOK {
		t.Fatalf("report should open right after the check, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDeleteExpiredAnonymousReports verifies that the cleanup removes only
// anonymous rows older than 24h and keeps everything else.
func TestDeleteExpiredAnonymousReports(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()

	oldAnon := repository.AnonymousRequesterPrefix + "203.0.113.30"
	newAnon := repository.AnonymousRequesterPrefix + "203.0.113.31"
	authUser := "0xBBB35Cc6634C0532925a3b844Bc9e7595f5B2a1"

	for _, requester := range []string{oldAnon, newAnon, authUser} {
		if err := env.repo.RecordCheck(ctx, requester, reportTestAddr, "evm", "hacked"); err != nil {
			t.Fatalf("RecordCheck %s: %v", requester, err)
		}
	}
	env.backdateChecks(t, oldAnon, 25)
	env.backdateChecks(t, authUser, 25*24)

	deleted, err := env.repo.DeleteExpiredAnonymousReports(ctx, time.Now().Add(-models.AnonymousReportTTL))
	if err != nil {
		t.Fatalf("DeleteExpiredAnonymousReports: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted row, got %d", deleted)
	}

	if last, _ := env.repo.GetLastReportAccess(ctx, oldAnon, reportTestAddr, "evm"); last != nil {
		t.Error("old anonymous row should be deleted")
	}
	if last, _ := env.repo.GetLastReportAccess(ctx, newAnon, reportTestAddr, "evm"); last == nil {
		t.Error("recent anonymous row should be kept")
	}
	if last, _ := env.repo.GetLastReportAccess(ctx, authUser, reportTestAddr, "evm"); last == nil {
		t.Error("authenticated history should never be deleted")
	}
}

// TestShareReport_RequiresAuth ensures only authenticated users can make
// reports public; anonymous users get 401.
func TestShareReport_RequiresAuth(t *testing.T) {
	env := setupReportTest(t)
	env.seedHackedWallet(t, reportTestAddr, "evm")

	router := gin.New()
	router.POST("/api/report/share", middleware.RequireAuth(), env.handler.ShareReport)

	body, _ := json.Marshal(models.ShareReportRequest{Address: reportTestAddr, Chain: "evm"})
	req := httptest.NewRequest("POST", "/api/report/share", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for anonymous share, got %d", w.Code)
	}
}

// TestShareReport_Flow covers minting a public UUID link and opening it
// without any authentication.
func TestShareReport_Flow(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()
	env.seedHackedWallet(t, reportTestAddr, "evm")

	userWallet := "0xCCC35Cc6634C0532925a3b844Bc9e7595f5B2a1"
	if err := env.repo.RecordCheck(ctx, userWallet, reportTestAddr, "evm", "hacked"); err != nil {
		t.Fatalf("RecordCheck: %v", err)
	}

	authRouter := gin.New()
	authRouter.Use(func(c *gin.Context) { c.Set("userAddress", userWallet) })
	authRouter.POST("/api/report/share", middleware.RequireAuth(), env.handler.ShareReport)

	body, _ := json.Marshal(models.ShareReportRequest{Address: reportTestAddr, Chain: "evm"})
	req := httptest.NewRequest("POST", "/api/report/share", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	authRouter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("share failed: %d: %s", w.Code, w.Body.String())
	}
	var share models.ShareReportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &share); err != nil {
		t.Fatalf("decode share response: %v", err)
	}
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !uuidRe.MatchString(share.ShareID) {
		t.Errorf("share_id is not a UUID: %q", share.ShareID)
	}
	if share.ShareURL != "/report/"+share.ShareID {
		t.Errorf("unexpected share_url: %q", share.ShareURL)
	}

	// the public link opens from a different IP without any token
	publicRouter := gin.New()
	publicRouter.GET("/api/report/shared/:id", env.handler.GetSharedReport)

	sharedReq := httptest.NewRequest("GET", "/api/report/shared/"+share.ShareID, nil)
	sharedReq.RemoteAddr = "198.51.100.99:1"
	sw := httptest.NewRecorder()
	publicRouter.ServeHTTP(sw, sharedReq)

	if sw.Code != http.StatusOK {
		t.Fatalf("shared report failed: %d: %s", sw.Code, sw.Body.String())
	}
	report := decodeReport(t, sw)
	if !report.Public {
		t.Error("shared report must be marked public")
	}
	if report.Address != reportTestAddr || report.Status != string(models.StatusHacked) {
		t.Errorf("unexpected shared report: %+v", report)
	}
	if report.Transactions == nil {
		t.Error("shared report must include the transaction tree")
	}

	// invalid tokens never resolve
	tampered := share.ShareID[:len(share.ShareID)-1]
	if share.ShareID[len(share.ShareID)-1] == '0' {
		tampered += "1"
	} else {
		tampered += "0"
	}
	for _, bad := range []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000", tampered} {
		badReq := httptest.NewRequest("GET", "/api/report/shared/"+bad, nil)
		bw := httptest.NewRecorder()
		publicRouter.ServeHTTP(bw, badReq)
		if bw.Code != http.StatusNotFound {
			t.Errorf("bad token %q: expected 404, got %d", bad, bw.Code)
		}
	}

	// sharing requires a previous check
	body2, _ := json.Marshal(models.ShareReportRequest{Address: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", Chain: "btc"})
	req2 := httptest.NewRequest("POST", "/api/report/share", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	authRouter.ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Errorf("share without prior check: expected 403, got %d", w2.Code)
	}
}

// TestGetReport_TreeMarksProgramNodes: a counterparty that is one of the
// on-chain programs invoked by the drainer transaction (not the hacker
// wallet) is shown as a program node and never expanded.
func TestGetReport_TreeMarksProgramNodes(t *testing.T) {
	env := setupReportTest(t)
	env.seedHackedWallet(t, reportTestAddr, "evm")

	_, inserted, err := env.repo.InsertScanFinding(context.Background(), models.ScanFindingRequest{
		Chain:         "solana",
		Signature:     "sig-prog-tree",
		Slot:          1,
		Verdict:       models.ScanVerdictDrainer,
		Indicators:    []string{"P1_ACCOUNT_TAKEOVER"},
		VictimAddress: reportTestAddr,
		HackerAddress: scanAddrSender3,
		AmountSOL:     1.2,
		Programs:      []string{scanAddrSender3},
		Source:        "test",
	})
	if err != nil || !inserted {
		t.Fatalf("InsertScanFinding: inserted=%v err=%v", inserted, err)
	}

	requester := repository.AnonymousRequesterPrefix + "203.0.113.21"
	if err := env.repo.RecordCheck(context.Background(), requester, reportTestAddr, "evm", "hacked"); err != nil {
		t.Fatalf("RecordCheck: %v", err)
	}

	router := gin.New()
	router.GET("/api/report", env.handler.GetReport)

	w := doReportRequest(router, reportTestAddr, "evm", "203.0.113.21:1")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	report := decodeReport(t, w)

	root := report.Transactions
	if root == nil || len(root.Children) != 1 {
		t.Fatalf("expected one child node, got %+v", root)
	}
	child := root.Children[0]
	if child.Address != scanAddrSender3 {
		t.Errorf("expected child %s, got %s", scanAddrSender3, child.Address)
	}
	if !child.IsProgram || child.Status != models.TreeStatusProgram {
		t.Errorf("program counterparty must be marked as program, got is_program=%v status=%q", child.IsProgram, child.Status)
	}
	if len(child.Children) != 0 {
		t.Errorf("program node without indexed payouts must have no children, got %d", len(child.Children))
	}
}

// TestGetReport_TreeExpandsProgramPayouts: a takeover-only finding names the
// owning on-chain program as the hacker (old findings don't list it under
// programs). The tree must show it as a program, never as a hacker, and
// expand it to the real payout recipients indexed by flow-trace findings:
// repeat recipients classify as hacker, one-off recipients as suspicious.
func TestGetReport_TreeExpandsProgramPayouts(t *testing.T) {
	env := setupReportTest(t)
	ctx := context.Background()
	env.seedHackedWallet(t, reportTestAddr, "evm")

	program := scanAddrSender3
	// Old-shape takeover finding: hacker is the takeover program, programs empty.
	if _, inserted, err := env.repo.InsertScanFinding(ctx, models.ScanFindingRequest{
		Chain:         "solana",
		Signature:     "sig-takeover-prog",
		Slot:          1,
		Verdict:       models.ScanVerdictDrainer,
		Indicators:    []string{"P1_ACCOUNT_TAKEOVER"},
		VictimAddress: reportTestAddr,
		HackerAddress: program,
		AmountSOL:     0,
		Source:        "test",
	}); err != nil || !inserted {
		t.Fatalf("InsertScanFinding takeover: inserted=%v err=%v", inserted, err)
	}

	seedFlow := func(sig, recipient string, amount float64, indicator string) {
		t.Helper()
		if _, inserted, err := env.repo.InsertScanFinding(ctx, models.ScanFindingRequest{
			Chain:            "solana",
			Signature:        sig,
			Slot:             1,
			Verdict:          models.ScanVerdictDrainer,
			Indicators:       []string{indicator},
			HackerAddress:    recipient,
			AmountSOL:        amount,
			ExposedAddresses: []string{program},
			Source:           "flow-trace",
		}); err != nil || !inserted {
			t.Fatalf("InsertScanFinding %s: inserted=%v err=%v", sig, inserted, err)
		}
	}
	// Registered in wallets: F2 recipient is a known hacker.
	if _, err := env.repo.CreateWallet(ctx, scanAddrHacker, "solana", models.StatusHacker, "", ""); err != nil {
		t.Fatalf("CreateWallet hacker: %v", err)
	}
	seedFlow("sig-flow-f2", scanAddrHacker, 5.0, "F2_REPEAT_DOWNSTREAM")
	// Not registered: one-off recipient must classify as suspicious…
	seedFlow("sig-flow-f1a", scanAddrSender4, 1.0, "F1_DOWNSTREAM_TRANSFER")
	// …and a recipient paid in two separate findings is recurring => hacker.
	seedFlow("sig-flow-r1", scanAddrSender1, 2.0, "F1_DOWNSTREAM_TRANSFER")
	seedFlow("sig-flow-r2", scanAddrSender1, 3.0, "F1_DOWNSTREAM_TRANSFER")

	requester := repository.AnonymousRequesterPrefix + "203.0.113.22"
	if err := env.repo.RecordCheck(ctx, requester, reportTestAddr, "evm", "hacked"); err != nil {
		t.Fatalf("RecordCheck: %v", err)
	}

	router := gin.New()
	router.GET("/api/report", env.handler.GetReport)

	w := doReportRequest(router, reportTestAddr, "evm", "203.0.113.22:1")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	report := decodeReport(t, w)

	root := report.Transactions
	if root == nil || len(root.Children) != 1 {
		t.Fatalf("expected the program as the single root child, got %+v", root)
	}
	prog := root.Children[0]
	if prog.Address != program || !prog.IsProgram || prog.Status != models.TreeStatusProgram {
		t.Fatalf("root child must be the program node, got %+v", prog)
	}

	// The program node expands to payout recipients only — not to the victims
	// drained through it.
	if len(prog.Children) != 3 {
		t.Fatalf("program node must list 3 payout recipients, got %+v", prog.Children)
	}
	byAddr := map[string]*models.ReportTxNode{}
	for _, ch := range prog.Children {
		byAddr[ch.Address] = ch
	}
	if ch := byAddr[scanAddrHacker]; ch == nil || ch.Status != string(models.StatusHacker) {
		t.Errorf("F2 recipient registered as hacker must show hacker, got %+v", ch)
	}
	if ch := byAddr[scanAddrSender4]; ch == nil || ch.Status != string(models.StatusSuspicious) {
		t.Errorf("one-off payout recipient must show suspicious, got %+v", ch)
	}
	if ch := byAddr[scanAddrSender1]; ch == nil || ch.Status != string(models.StatusHacker) {
		t.Errorf("recurring payout recipient (2 findings) must show hacker, got %+v", ch)
	}
	if ch := byAddr[scanAddrHacker]; ch != nil && ch.Amount != 5.0 {
		t.Errorf("hacker payout amount must aggregate to 5.0, got %f", ch.Amount)
	}

	// The funding sources must round-trip through storage.
	findings, err := env.repo.GetFlowPayoutsForSource(ctx, program, 10)
	if err != nil || len(findings) != 4 {
		t.Fatalf("GetFlowPayoutsForSource: %d findings, err=%v", len(findings), err)
	}
	if len(findings[0].ExposedAddresses) != 1 || findings[0].ExposedAddresses[0] != program {
		t.Errorf("exposed addresses not persisted: %+v", findings[0].ExposedAddresses)
	}
}
