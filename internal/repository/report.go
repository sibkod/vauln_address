package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"vauln-address/internal/models"
)

// AnonymousRequesterPrefix marks check_history rows created by
// non-authenticated users: wallet_address holds "ip:<ip>" instead of a wallet.
const AnonymousRequesterPrefix = "ip:"

// GetWalletReport reads the wallet row together with the existing
// reason/source columns (migration 004). Returns nil when not found.
func (r *Repository) GetWalletReport(ctx context.Context, address, chain string) (*models.ReportResponse, error) {
	lookupAddr := normalizeAddress(chain, address)
	var (
		status           string
		hasPK, hasSeed   bool
		associatedHacker bool
		reason, source   sql.NullString
		associatedReason sql.NullString
		createdAt        time.Time
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT status, has_pk, has_seed, reason, source, created_at,
			COALESCE(associated_hacker, false), COALESCE(associated_reason, '')
			FROM wallets
			WHERE address = ? AND chain = ?`,
		lookupAddr, chain,
	).Scan(&status, &hasPK, &hasSeed, &reason, &source, &createdAt,
		&associatedHacker, &associatedReason)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &models.ReportResponse{
		Address:          address,
		Chain:            chain,
		Found:            true,
		Status:           status,
		Reason:           reason.String,
		Source:           source.String,
		HasPK:            hasPK,
		HasSeed:          hasSeed,
		AssociatedHacker: associatedHacker,
		AssociatedReason: associatedReason.String,
		CreatedAt:        createdAt,
	}, nil
}

// GetLeaksForAddress returns leak metadata (never key values) for an address.
// A missing leaked_keys table is treated as "no leaks".
func (r *Repository) GetLeaksForAddress(ctx context.Context, address, chain string) ([]models.LeakedKeyInfo, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT key_type, source, discovered_at
			FROM leaked_keys
			WHERE address = ? AND chain = ?
			ORDER BY discovered_at DESC`,
		address, chain,
	)
	if err != nil {
		if isMissingTableErr(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var leaks []models.LeakedKeyInfo
	for rows.Next() {
		var leak models.LeakedKeyInfo
		var source sql.NullString
		if err := rows.Scan(&leak.KeyType, &source, &leak.DiscoveredAt); err != nil {
			return nil, err
		}
		leak.Source = source.String
		leaks = append(leaks, leak)
	}
	return leaks, rows.Err()
}

// GetWalletStatus resolves the status of a single address for the
// transaction tree. Returns "" when the address is not in the database.
func (r *Repository) GetWalletStatus(ctx context.Context, address, chain string) (string, error) {
	lookupAddr := normalizeAddress(chain, address)
	var status string
	err := r.db.QueryRowContext(ctx,
		`SELECT status FROM wallets WHERE address = ? AND chain = ?`,
		lookupAddr, chain,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return status, nil
}

// GetWalletAssociation returns whether the address is flagged as associated
// with a hacker operator. False when the address is not in the database.
func (r *Repository) GetWalletAssociation(ctx context.Context, address, chain string) bool {
	var flag bool
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(associated_hacker, false) FROM wallets WHERE address = ? AND chain = ?`,
		address, chain,
	).Scan(&flag)
	if err != nil {
		return false
	}
	return flag
}

// GetLastReportAccess returns when the requester last checked the address.
// The requester is the authenticated wallet address or "ip:<ip>" for
// anonymous users; both live in the existing check_history.wallet_address
// column. Returns nil when the requester never checked this address.
func (r *Repository) GetLastReportAccess(ctx context.Context, requester, address, chain string) (*time.Time, error) {
	var checkedAt time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT created_at FROM check_history
			WHERE wallet_address = ? AND address = ? AND chain = ?
			ORDER BY created_at DESC LIMIT 1`,
		requester, address, chain,
	).Scan(&checkedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &checkedAt, nil
}

// GetReportCheckRow returns the id and timestamp of the requester's latest
// check of the address. The id backs the public share token of the report.
func (r *Repository) GetReportCheckRow(ctx context.Context, requester, address, chain string) (int64, time.Time, bool, error) {
	var (
		id        int64
		checkedAt time.Time
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT id, created_at FROM check_history
			WHERE wallet_address = ? AND address = ? AND chain = ?
			ORDER BY created_at DESC LIMIT 1`,
		requester, address, chain,
	).Scan(&id, &checkedAt)
	if err == sql.ErrNoRows {
		return 0, time.Time{}, false, nil
	}
	if err != nil {
		return 0, time.Time{}, false, err
	}
	return id, checkedAt, true, nil
}

// GetReportCheckByID resolves a check_history row back to the checked
// address and chain. Used to open publicly shared reports.
func (r *Repository) GetReportCheckByID(ctx context.Context, id int64) (string, string, error) {
	var address, chain string
	err := r.db.QueryRowContext(ctx,
		`SELECT address, chain FROM check_history WHERE id = ?`, id,
	).Scan(&address, &chain)
	if err != nil {
		return "", "", err
	}
	return address, chain, nil
}

// DeleteExpiredAnonymousReports removes anonymous check history older than
// the cutoff, which effectively deletes anonymous reports (their only access
// record). Authenticated history rows are kept forever.
func (r *Repository) DeleteExpiredAnonymousReports(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM check_history WHERE wallet_address LIKE ? AND created_at < ?`,
		AnonymousRequesterPrefix+"%", cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func isMissingTableErr(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") || // postgres
		strings.Contains(msg, "no such table") || // sqlite
		strings.Contains(msg, "doesn't exist") // mysql
}
