package repository

import (
	"context"
	"sync"
	"testing"

	"vauln-address/internal/config"
	"vauln-address/internal/models"
)

func setupScanTest(t *testing.T) *Repository {
	t.Helper()
	cfg := &config.Config{
		DBType:         config.DBTypeSQLite,
		SQLitePath:     t.TempDir() + "/scan_test.db",
		FreeCheckLimit: 3,
	}
	repo, err := New(cfg)
	if err != nil {
		t.Fatalf("repository.New: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	return repo
}

func TestInsertScanFinding_Dedup(t *testing.T) {
	repo := setupScanTest(t)
	ctx := context.Background()
	req := models.ScanFindingRequest{
		Chain:     "solana",
		Signature: "sig-dup-1",
		Verdict:   models.ScanVerdictDrainer,
	}

	id1, inserted, err := repo.InsertScanFinding(ctx, req)
	if err != nil || !inserted {
		t.Fatalf("first insert: id=%d inserted=%v err=%v", id1, inserted, err)
	}
	id2, inserted, err := repo.InsertScanFinding(ctx, req)
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if inserted || id2 != id1 {
		t.Fatalf("duplicate must return (id=%d, false), got (id=%d, %v)", id1, id2, inserted)
	}
}

// Concurrent ingests of the same signature (multithreaded scanner): exactly
// one request wins the insert, the rest get the unique-violation fallback
// and must report (same id, false) — never an error.
func TestInsertScanFinding_ConcurrentDedup(t *testing.T) {
	repo := setupScanTest(t)
	ctx := context.Background()
	req := models.ScanFindingRequest{
		Chain:     "solana",
		Signature: "sig-race-1",
		Verdict:   models.ScanVerdictDrainer,
	}

	const workers = 16
	var wg sync.WaitGroup
	results := make([]struct {
		id       int64
		inserted bool
		err      error
	}, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, inserted, err := repo.InsertScanFinding(ctx, req)
			results[i].id, results[i].inserted, results[i].err = id, inserted, err
		}(i)
	}
	wg.Wait()

	insertedCount := 0
	var winnerID int64
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("worker %d got error: %v", i, r.err)
		}
		if r.inserted {
			insertedCount++
			winnerID = r.id
		}
	}
	if insertedCount != 1 {
		t.Fatalf("exactly one insert must win, got %d", insertedCount)
	}
	for i, r := range results {
		if r.id != winnerID {
			t.Fatalf("worker %d: id=%d, want winner id=%d", i, r.id, winnerID)
		}
	}

	var rowCount int
	if err := repo.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scan_findings WHERE signature = ?`,
		req.Signature).Scan(&rowCount); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("expected 1 row for signature, got %d", rowCount)
	}
}

// The per-recipient breakdown of a split drain is persisted and every
// recipient (not only victim/hacker) finds the finding by its address.
func TestScanFindingSweeps_Persistence(t *testing.T) {
	repo := setupScanTest(t)
	ctx := context.Background()
	req := models.ScanFindingRequest{
		Chain:         "solana",
		Signature:     "sig-sweeps-1",
		Verdict:       models.ScanVerdictDrainer,
		VictimAddress: "victimA",
		HackerAddress: "hackerA",
		AmountSOL:     1.4962,
		Sweeps: []models.SweepTransfer{
			{Address: "hackerA", AmountSOL: 0.7481},
			{Address: "recipientB", AmountSOL: 0.7481},
		},
	}
	if _, inserted, err := repo.InsertScanFinding(ctx, req); err != nil || !inserted {
		t.Fatalf("insert: inserted=%v err=%v", inserted, err)
	}

	for _, addr := range []string{"victimA", "hackerA", "recipientB"} {
		findings, err := repo.GetScanFindingsForAddress(ctx, addr, 10)
		if err != nil {
			t.Fatalf("GetScanFindingsForAddress %s: %v", addr, err)
		}
		if len(findings) != 1 {
			t.Fatalf("%s must see the finding, got %d", addr, len(findings))
		}
		sweeps := findings[0].Sweeps
		if len(sweeps) != 2 {
			t.Fatalf("sweeps must round-trip, got %+v", sweeps)
		}
		if sweeps[0].Address != "hackerA" || sweeps[0].AmountSOL != 0.7481 ||
			sweeps[1].Address != "recipientB" || sweeps[1].AmountSOL != 0.7481 {
			t.Errorf("unexpected sweeps: %+v", sweeps)
		}
	}

	// a prefix of a recipient address must not match the CSV element
	findings, err := repo.GetScanFindingsForAddress(ctx, "recipient", 10)
	if err != nil {
		t.Fatalf("GetScanFindingsForAddress prefix: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("prefix of a sweep recipient must not match, got %d findings", len(findings))
	}
}

// CreateWallet on an already-registered address returns the existing row id
// without an error instead of inserting a duplicate (the wallets table has
// no UNIQUE(chain,address) to fall back on).
func TestCreateWallet_DuplicateReturnsExisting(t *testing.T) {
	repo := setupScanTest(t)
	ctx := context.Background()

	id1, err := repo.CreateWallet(ctx, "dupAddr1", "solana", models.StatusSuspicious, "first", "test")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	id2, err := repo.CreateWallet(ctx, "dupAddr1", "solana", models.StatusHacker, "second", "test")
	if err != nil {
		t.Fatalf("duplicate create must not error: %v", err)
	}
	if id2 != id1 {
		t.Fatalf("duplicate must return existing id %d, got %d", id1, id2)
	}

	// the original status wins; no second row appears
	var count int
	if err := repo.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM wallets WHERE address = ? AND chain = ?`,
		"dupAddr1", "solana").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 wallet row, got %d", count)
	}
}

// Concurrent creation of the same wallet (parallel scanner ingest): exactly
// one row exists afterwards and every caller got a valid id.
func TestCreateWallet_ConcurrentNoDuplicates(t *testing.T) {
	repo := setupScanTest(t)
	ctx := context.Background()

	const workers = 16
	var wg sync.WaitGroup
	ids := make([]int64, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ids[i], errs[i] = repo.CreateWallet(ctx, "raceAddr1", "solana", models.StatusSuspicious, "race", "test")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
		if ids[i] == 0 {
			t.Fatalf("worker %d: got zero id", i)
		}
	}

	var count int
	if err := repo.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM wallets WHERE address = ? AND chain = ?`,
		"raceAddr1", "solana").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 wallet row, got %d", count)
	}
}

// Concurrent association marking of an unseen wallet: exactly one row is
// created even though every caller takes the update-then-insert path.
func TestMarkAssociatedHacker_ConcurrentNoDuplicates(t *testing.T) {
	repo := setupScanTest(t)
	ctx := context.Background()

	const workers = 16
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = repo.MarkAssociatedHacker(ctx, "assocRace1", "solana", "funded operator X")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}

	var count int
	var assoc bool
	if err := repo.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(MAX(associated_hacker), 0) FROM wallets WHERE address = ? AND chain = ?`,
		"assocRace1", "solana").Scan(&count, &assoc); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 wallet row, got %d", count)
	}
	if !assoc {
		t.Fatalf("wallet must be flagged as associated")
	}
}
