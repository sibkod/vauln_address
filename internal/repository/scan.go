package repository

import (
	"context"
	"database/sql"
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

// InsertScanFinding stores one scanner detection. The signature is unique,
// so duplicates are skipped (second return value = false).
func (r *Repository) InsertScanFinding(ctx context.Context, req models.ScanFindingRequest) (int64, bool, error) {
	chain := req.Chain
	if chain == "" {
		chain = "solana"
	}
	// dedupe by signature; the UNIQUE constraint backs this check
	var existing int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM scan_findings WHERE signature = ?`, req.Signature).Scan(&existing)
	if err == nil {
		return existing, false, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}

	res, err := r.db.ExecContext(ctx,
		`INSERT INTO scan_findings
		 (chain, signature, slot, verdict, indicators, victim_address,
		  hacker_address, amount_sol, programs, exposed_addresses, source, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		chain, req.Signature, req.Slot, req.Verdict,
		joinTags(req.Indicators), req.VictimAddress, req.HackerAddress,
		req.AmountSOL, joinTags(req.Programs), joinTags(req.ExposedAddresses),
		req.Source, time.Now().UTC(),
	)
	if err != nil {
		return 0, false, err
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
	var indicators, programs, victim, hacker, exposed, source sql.NullString
	err := scan(&f.ID, &f.Chain, &f.Signature, &f.Slot, &f.Verdict,
		&indicators, &victim, &hacker, &f.AmountSOL, &programs, &exposed,
		&source, &f.CreatedAt)
	if err != nil {
		return nil, err
	}
	f.Indicators = splitTags(indicators.String)
	f.Programs = splitTags(programs.String)
	f.VictimAddress = victim.String
	f.HackerAddress = hacker.String
	f.ExposedAddresses = splitTags(exposed.String)
	f.Source = source.String
	return &f, nil
}

const scanFindingCols = `id, chain, signature, slot, verdict, indicators,
	victim_address, hacker_address, amount_sol, programs, exposed_addresses, source, created_at`

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
// victim or the hacker (powers the report evidence chain).
func (r *Repository) GetScanFindingsForAddress(ctx context.Context, address string, limit int) ([]models.ScanFinding, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+scanFindingCols+` FROM scan_findings
		 WHERE victim_address = ? OR hacker_address = ?
		 ORDER BY id DESC LIMIT ?`,
		address, address, limit,
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
