package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vauln-address/internal/config"
	"vauln-address/internal/models"
)

type Repository struct {
	db *pgxpool.Pool
}

func New(cfg *config.Config) (*Repository, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBSSLMode,
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	poolConfig.MaxConns = 10
	poolConfig.MinConns = 2

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Repository{db: pool}, nil
}

func (r *Repository) Close() {
	r.db.Close()
}

func (r *Repository) InitSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS wallets (
		id BIGSERIAL PRIMARY KEY,
		address VARCHAR(100) NOT NULL,
		chain VARCHAR(20) NOT NULL,
		status VARCHAR(20) NOT NULL,
		has_pk BOOLEAN DEFAULT FALSE,
		has_seed BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_wallets_address ON wallets(address);
	CREATE INDEX IF NOT EXISTS idx_wallets_chain ON wallets(chain);
	CREATE INDEX IF NOT EXISTS idx_wallets_status ON wallets(status);
	CREATE INDEX IF NOT EXISTS idx_wallets_chain_status ON wallets(chain, status);

	CREATE TABLE IF NOT EXISTS contact_messages (
		id BIGSERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		email VARCHAR(255) NOT NULL,
		message TEXT NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS rate_limits (
		id BIGSERIAL PRIMARY KEY,
		ip_address VARCHAR(45) NOT NULL UNIQUE,
		request_count INTEGER DEFAULT 0,
		window_start TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_rate_limits_ip ON rate_limits(ip_address);
	`

	_, err := r.db.Exec(ctx, schema)
	return err
}

func (r *Repository) GetWallet(ctx context.Context, address string, chain string) (*models.Wallet, error) {
	var wallet models.Wallet
	err := r.db.QueryRow(ctx,
		`SELECT id, address, chain, status, has_pk, has_seed, created_at, updated_at 
		FROM wallets 
		WHERE address = $1 AND chain = $2`,
		address, chain,
	).Scan(&wallet.ID, &wallet.Address, &wallet.Chain, &wallet.Status,
		&wallet.HasPK, &wallet.HasSeed, &wallet.CreatedAt, &wallet.UpdatedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &wallet, nil
}

func (r *Repository) SaveContactMessage(ctx context.Context, msg *models.ContactMessage) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO contact_messages (name, email, message) VALUES ($1, $2, $3)`,
		msg.Name, msg.Email, msg.Message,
	)
	return err
}

func (r *Repository) GetRecentChecks(ctx context.Context, limit int) ([]models.RecentCheck, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, address, chain, status, created_at 
		FROM wallets 
		WHERE status IN ('hacked', 'vulnerable', 'hacker') 
		ORDER BY created_at DESC 
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []models.RecentCheck
	for rows.Next() {
		var check models.RecentCheck
		if err := rows.Scan(&check.ID, &check.Address, &check.Chain, &check.Status, &check.CheckedAt); err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}
	return checks, rows.Err()
}

func (r *Repository) GetRateLimit(ctx context.Context, ip string) (*models.RateLimit, error) {
	var rl models.RateLimit
	err := r.db.QueryRow(ctx,
		`SELECT id, ip_address, request_count, window_start 
		FROM rate_limits 
		WHERE ip_address = $1`,
		ip,
	).Scan(&rl.ID, &rl.IPAddress, &rl.Count, &rl.WindowStart)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rl, nil
}

func (r *Repository) IncrementRateLimit(ctx context.Context, ip string, windowStart time.Time) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO rate_limits (ip_address, request_count, window_start) 
		VALUES ($1, 1, $2) 
		ON CONFLICT (ip_address) 
		DO UPDATE SET 
			request_count = rate_limits.request_count + 1,
			window_start = CASE 
				WHEN rate_limits.window_start < $2 THEN $2 
				ELSE rate_limits.window_start 
			END`,
		ip, windowStart,
	)
	return err
}

func (r *Repository) ResetRateLimit(ctx context.Context, ip string, windowStart time.Time) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO rate_limits (ip_address, request_count, window_start) 
		VALUES ($1, 1, $2) 
		ON CONFLICT (ip_address) 
		DO UPDATE SET request_count = 1, window_start = $2`,
		ip, windowStart,
	)
	return err
}

func (r *Repository) RecordCheck(ctx context.Context, address, chain, status string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO check_history (address, chain, status) VALUES ($1, $2, $3)`,
		address, chain, status,
	)
	return err
}
