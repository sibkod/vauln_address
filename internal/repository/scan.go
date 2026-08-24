package repository

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"vauln-address/internal/config"
	"vauln-address/internal/models"
)

// splitJoin converts a []string to a comma-joined DB value and back.
// Scanner indicators and program lists are short tags, so CSV is enough.
func joinTags(tags []string) string {
	return strings.Join(tags, ",")
}

func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	out := strings.Split(s, ",")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

// joinSweeps / splitSweeps convert the per-recipient sweep breakdown to a
// CSV of "address:amount_sol" pairs and back. Base58/hex addresses never
// contain ':' or ',', so the format is unambiguous and stays searchable
// with the same whole-element LIKE trick as exposed_addresses.
func joinSweeps(sweeps []models.SweepTransfer) string {
	parts := make([]string, 0, len(sweeps))
	for _, sw := range sweeps {
		if sw.Address == "" {
			continue
		}
		parts = append(parts, sw.Address+":"+strconv.FormatFloat(sw.AmountSOL, 'f', -1, 64))
	}
	return strings.Join(parts, ",")
}

func splitSweeps(s string) []models.SweepTransfer {
	if s == "" {
		return nil
	}
	var out []models.SweepTransfer
	for _, part := range strings.Split(s, ",") {
		addr, amount, found := strings.Cut(part, ":")
		if !found || addr == "" {
			continue
		}
		v, err := strconv.ParseFloat(amount, 64)
		if err != nil {
			continue
		}
		out = append(out, models.SweepTransfer{Address: addr, AmountSOL: v})
	}
	return out
}

// isUniqueViolation reports whether err is a duplicate-key error from any
// supported driver (sqlite3 / mysql / postgres). String matching avoids
// importing driver-specific error types.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || // sqlite3
		strings.Contains(msg, "Duplicate entry") || // mysql 1062
		strings.Contains(msg, "duplicate key value") // postgres 23505
}

// InsertScanFinding stores one scanner detection. The signature is unique,
// so duplicates are skipped (second return value = false). The insert is a
// single round-trip per dialect (INSERT IGNORE / OR IGNORE / ON CONFLICT):
// a concurrent ingest of the same signature (e.g. from the multithreaded
// scanner) reports the existing row as a duplicate instead of an error.
func (r *Repository) InsertScanFinding(ctx context.Context, req models.ScanFindingRequest) (int64, bool, error) {
	chain := req.Chain
	if chain == "" {
		chain = "solana"
	}
	args := []interface{}{
		chain, req.Signature, req.Slot, req.Verdict,
		joinTags(req.Indicators), req.VictimAddress, req.HackerAddress,
		req.AmountSOL, joinTags(req.Programs), joinSweeps(req.Sweeps),
		joinTags(req.ExposedAddresses),
		req.Source, time.Now().UTC(),
	}

	// duplicate resolves the row that won the unique constraint
	duplicate := func() (int64, bool, error) {
		var existing int64
		if err := r.db.QueryRowContext(ctx,
			`SELECT id FROM scan_findings WHERE signature = ?`,
			req.Signature).Scan(&existing); err != nil {
			return 0, false, err
		}
		return existing, false, nil
	}

	if r.dbType == config.DBTypePostgres {
		// pq/pgx have no LastInsertId: RETURNING yields no row on conflict.
		var id int64
		err := r.db.QueryRowContext(ctx,
			`INSERT INTO scan_findings
			 (chain, signature, slot, verdict, indicators, victim_address,
			  hacker_address, amount_sol, programs, sweeps, exposed_addresses, source, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (signature) DO NOTHING RETURNING id`,
			args...).Scan(&id)
		if err == sql.ErrNoRows {
			return duplicate()
		}
		if err != nil {
			return 0, false, err
		}
		return id, true, nil
	}

	insert := `INSERT INTO scan_findings
	 (chain, signature, slot, verdict, indicators, victim_address,
	  hacker_address, amount_sol, programs, sweeps, exposed_addresses, source, created_at)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if r.dbType == config.DBTypeMySQL {
		insert = strings.Replace(insert, "INSERT INTO", "INSERT IGNORE INTO", 1)
	} else {
		insert = strings.Replace(insert, "INSERT INTO", "INSERT OR IGNORE INTO", 1)
	}
	res, err := r.db.ExecContext(ctx, insert, args...)
	if err != nil {
		if isUniqueViolation(err) {
			// driver without IGNORE support surfaced the constraint
			return duplicate()
		}
		return 0, false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	if affected == 0 {
		return duplicate()
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// scanFindingScanner maps a scan_findings row to the model.
func scanFindingRow(scan func(dest ...interface{}) error) (*models.ScanFinding, error) {
	var f models.ScanFinding
	var indicators, programs, victim, hacker, sweeps, exposed, source sql.NullString
	err := scan(&f.ID, &f.Chain, &f.Signature, &f.Slot, &f.Verdict,
		&indicators, &victim, &hacker, &f.AmountSOL, &programs, &sweeps,
		&exposed, &source, &f.CreatedAt)
	if err != nil {
		return nil, err
	}
	f.Indicators = splitTags(indicators.String)
	f.Programs = splitTags(programs.String)
	f.VictimAddress = victim.String
	f.HackerAddress = hacker.String
	f.Sweeps = splitSweeps(sweeps.String)
	f.ExposedAddresses = splitTags(exposed.String)
	f.Source = source.String
	return &f, nil
}

const scanFindingCols = `id, chain, signature, slot, verdict, indicators,
	victim_address, hacker_address, amount_sol, programs, sweeps, exposed_addresses, source, created_at`

// GetScanFindings lists findings. With afterID > 0 it returns newer rows
// ascending (live polling); with beforeID > 0 it returns older rows
// descending (load-more pagination); otherwise the latest rows descending.
func (r *Repository) GetScanFindings(ctx context.Context, afterID, beforeID int64, limit int) ([]models.ScanFinding, error) {
	var rows *sql.Rows
	var err error
	switch {
	case afterID > 0:
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+scanFindingCols+` FROM scan_findings WHERE id > ? ORDER BY id ASC LIMIT ?`,
			afterID, limit)
	case beforeID > 0:
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+scanFindingCols+` FROM scan_findings WHERE id < ? ORDER BY id DESC LIMIT ?`,
			beforeID, limit)
	default:
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+scanFindingCols+` FROM scan_findings ORDER BY id DESC LIMIT ?`,
			limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ScanFinding
	for rows.Next() {
		f, err := scanFindingRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// GetScanFindingsForAddress returns findings where the address is the
// victim, the hacker, or one of the sweep recipients of a split drain
// (powers the report evidence chain).
func (r *Repository) GetScanFindingsForAddress(ctx context.Context, address string, limit int) ([]models.ScanFinding, error) {
	// sweeps is a CSV of "address:amount" pairs; match the address only as a
	// whole element (the trailing ':' also guards against prefix collisions).
	var sweepMatch string
	if r.dbType == config.DBTypeMySQL {
		sweepMatch = "CONCAT(',', COALESCE(sweeps, ''), ',') LIKE CONCAT('%,', ?, ':%')"
	} else {
		sweepMatch = "',' || COALESCE(sweeps, '') || ',' LIKE '%,' || ? || ':%'"
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+scanFindingCols+` FROM scan_findings
		 WHERE victim_address = ? OR hacker_address = ? OR `+sweepMatch+`
		 ORDER BY id DESC LIMIT ?`,
		address, address, address, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ScanFinding
	for rows.Next() {
		f, err := scanFindingRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// GetFlowPayoutsForSource returns flow-trace findings (F1/F2 indicators)
// whose funding sources include the given address — i.e. the payouts this
// address (operator wallet or drainer program) made to downstream wallets.
// The hacker_address of each returned finding is the payout recipient.
func (r *Repository) GetFlowPayoutsForSource(ctx context.Context, source string, limit int) ([]models.ScanFinding, error) {
	if source == "" {
		return nil, nil
	}
	// exposed_addresses is a CSV; match the address only as a whole element.
	var contains string
	if r.dbType == config.DBTypeMySQL {
		contains = "CONCAT(',', COALESCE(exposed_addresses, ''), ',') LIKE CONCAT('%,', ?, ',%')"
	} else {
		contains = "',' || COALESCE(exposed_addresses, '') || ',' LIKE '%,' || ? || ',%'"
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+scanFindingCols+` FROM scan_findings
		 WHERE (indicators LIKE '%F1_DOWNSTREAM_TRANSFER%'
		    OR indicators LIKE '%F2_REPEAT_DOWNSTREAM%')
		   AND `+contains+`
		 ORDER BY id DESC LIMIT ?`,
		source, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ScanFinding
	for rows.Next() {
		f, err := scanFindingRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// GetScanStats aggregates counters for the live monitoring page.
func (r *Repository) GetScanStats(ctx context.Context) (*models.ScanStats, error) {
	stats := &models.ScanStats{}
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN verdict = 'DRAINER' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN verdict = 'SUSPICIOUS' THEN 1 ELSE 0 END), 0),
			(SELECT COALESCE(SUM(amount_sol), 0) FROM scan_findings),
			(SELECT COUNT(DISTINCT victim_address) FROM scan_findings WHERE victim_address IS NOT NULL AND victim_address != ''),
			(SELECT COUNT(DISTINCT hacker_address) FROM scan_findings WHERE hacker_address IS NOT NULL AND hacker_address != '')
		 FROM scan_findings`,
	).Scan(&stats.TotalFindings, &stats.DrainerCount, &stats.SuspectCount,
		&stats.StolenSOL, &stats.VictimCount, &stats.HackerCount)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// InsertDrainerReport stores a user-submitted drainer report.
func (r *Repository) InsertDrainerReport(ctx context.Context, rep *models.DrainerReport) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO drainer_reports
		 (tx_signature, chain, site_url, description, reporter, status, telegram_sent, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rep.TxSignature, rep.Chain, rep.SiteURL, rep.Description,
		rep.Reporter, rep.Status, rep.TelegramSent, time.Now().UTC(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// MarkDrainerReportTelegram updates the telegram-sent flag after dispatch.
func (r *Repository) MarkDrainerReportTelegram(ctx context.Context, id int64, sent bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE drainer_reports SET telegram_sent = ? WHERE id = ?`, sent, id)
	return err
}

// InsertBugReport stores a user-submitted report about a wrong result.
func (r *Repository) InsertBugReport(ctx context.Context, rep *models.BugReport) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO bug_reports
		 (address, chain, message, reporter, status, telegram_sent, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rep.Address, rep.Chain, rep.Message,
		rep.Reporter, rep.Status, rep.TelegramSent, time.Now().UTC(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// MarkBugReportTelegram updates the telegram-sent flag after dispatch.
func (r *Repository) MarkBugReportTelegram(ctx context.Context, id int64, sent bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE bug_reports SET telegram_sent = ? WHERE id = ?`, sent, id)
	return err
}

// InsertLeakReport stores a user-submitted leak report. Only the secret
// fingerprint is persisted — never the secret itself.
func (r *Repository) InsertLeakReport(ctx context.Context, rep *models.LeakReport) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO leak_reports
		 (chain, secret_type, secret_hash, description, reporter, status, telegram_sent, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rep.Chain, rep.SecretType, rep.SecretHash, rep.Description,
		rep.Reporter, rep.Status, rep.TelegramSent, time.Now().UTC(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// MarkLeakReportTelegram updates the telegram-sent flag after dispatch.
func (r *Repository) MarkLeakReportTelegram(ctx context.Context, id int64, sent bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE leak_reports SET telegram_sent = ? WHERE id = ?`, sent, id)
	return err
}

// LeakReportHashSeen reports whether a secret fingerprint was already
// submitted — used for dedup and by tests to verify only the hash is stored.
func (r *Repository) LeakReportHashSeen(ctx context.Context, hash string) (bool, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM leak_reports WHERE secret_hash = ?`, hash).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
