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
