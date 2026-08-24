package repository

import (
	"context"
	"testing"

	"vauln-address/internal/models"
)

func TestBatchAddWallets_DedupAndMultiRow(t *testing.T) {
	repo := setupScanTest(t)
	ctx := context.Background()
	if err := repo.InitStatsTable(ctx); err != nil {
		t.Fatalf("InitStatsTable: %v", err)
	}

	// pre-existing wallet must be skipped
	if _, err := repo.CreateWallet(ctx, "existing1", "solana", models.StatusSafe, "seed", "test"); err != nil {
		t.Fatalf("seed existing: %v", err)
	}

	items := []BatchWalletItem{
		{Address: "new1", Chain: "solana", Status: models.StatusSuspicious, Reason: "r", Source: "s"},
		{Address: "new2", Chain: "solana", Status: models.StatusSuspicious, Reason: "r", Source: "s"},
		{Address: "new1", Chain: "solana", Status: models.StatusSuspicious, Reason: "r", Source: "s"}, // in-batch dup
		{Address: "existing1", Chain: "solana", Status: models.StatusSuspicious, Reason: "r", Source: "s"}, // db dup
		{Address: "new3", Chain: "solana", Status: models.StatusSuspicious, Reason: "r", Source: "s"},
	}
	_, results, err := repo.BatchAddWallets(ctx, items)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(results) != len(items) {
		t.Fatalf("results len %d, want %d", len(results), len(items))
	}
	if results[0].Skipped || results[0].WalletID == 0 {
		t.Errorf("new1 must be inserted: %+v", results[0])
	}
	if results[1].Skipped || results[1].WalletID == 0 {
		t.Errorf("new2 must be inserted: %+v", results[1])
	}
	if !results[2].Skipped {
		t.Errorf("in-batch dup must be skipped: %+v", results[2])
	}
	if !results[3].Skipped {
		t.Errorf("existing1 must be skipped: %+v", results[3])
	}
	if results[4].Skipped || results[4].WalletID == 0 {
		t.Errorf("new3 must be inserted: %+v", results[4])
	}

	var count int
	if err := repo.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM wallets WHERE chain = 'solana'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 4 { // existing1 + new1 + new2 + new3
		t.Fatalf("expected 4 wallet rows, got %d", count)
	}
}
