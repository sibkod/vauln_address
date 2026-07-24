package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	-- Wallets table
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

	-- Users table (Web3 authenticated)
	CREATE TABLE IF NOT EXISTS users (
		id BIGSERIAL PRIMARY KEY,
		wallet_address VARCHAR(100) NOT NULL,
		chain VARCHAR(20) NOT NULL,
		nonce VARCHAR(100),
		balance INTEGER DEFAULT 10,
		is_premium BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		last_login_at TIMESTAMP WITH TIME ZONE,
		UNIQUE(wallet_address, chain)
	);

	CREATE INDEX IF NOT EXISTS idx_users_wallet ON users(wallet_address);

	-- Orders table
	CREATE TABLE IF NOT EXISTS orders (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT REFERENCES users(id),
		order_uuid VARCHAR(100) UNIQUE NOT NULL,
		checks_count INTEGER NOT NULL,
		total_usd DECIMAL(10, 2) NOT NULL,
		currency VARCHAR(20) NOT NULL,
		token_amount DECIMAL(20, 8),
		payment_address VARCHAR(200),
		status VARCHAR(20) DEFAULT 'pending',
		tx_hash VARCHAR(200),
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		completed_at TIMESTAMP WITH TIME ZONE
	);

	CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(user_id);
	CREATE INDEX IF NOT EXISTS idx_orders_uuid ON orders(order_uuid);
	CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);

	-- Contact messages table
	CREATE TABLE IF NOT EXISTS contact_messages (
		id BIGSERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		email VARCHAR(255) NOT NULL,
		message TEXT NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	);

	-- Rate limits table
	CREATE TABLE IF NOT EXISTS rate_limits (
		id BIGSERIAL PRIMARY KEY,
		ip_address VARCHAR(45) NOT NULL UNIQUE,
		request_count INTEGER DEFAULT 0,
		window_start TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_rate_limits_ip ON rate_limits(ip_address);

	-- Check history table
	CREATE TABLE IF NOT EXISTS check_history (
		id BIGSERIAL PRIMARY KEY,
		address VARCHAR(100) NOT NULL,
		chain VARCHAR(20) NOT NULL,
		status VARCHAR(20),
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_check_history_created ON check_history(created_at);
	`

	_, err := r.db.Exec(ctx, schema)
	return err
}

// ==================== User Methods ====================

func (r *Repository) GetUserByWallet(ctx context.Context, address, chain string) (*models.User, error) {
	var user models.User
	err := r.db.QueryRow(ctx,
		`SELECT id, wallet_address, chain, nonce, balance, created_at, updated_at, last_login_at 
		FROM users WHERE wallet_address = $1 AND chain = $2`,
		address, chain,
	).Scan(&user.ID, &user.WalletAddress, &user.Chain, &user.Nonce, &user.Balance,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	var user models.User
	err := r.db.QueryRow(ctx,
		`SELECT id, wallet_address, chain, nonce, balance, created_at, updated_at, last_login_at 
		FROM users WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.WalletAddress, &user.Chain, &user.Nonce, &user.Balance,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetOrCreateUser(address, chain string) (*models.User, error) {
	ctx := context.Background()
	
	// Try to get existing user
	user, err := r.GetUserByWallet(ctx, address, chain)
	if err != nil {
		return nil, err
	}
	if user != nil {
		return user, nil
	}

	// Create new user with 10 free checks
	_, err = r.db.Exec(ctx,
		`INSERT INTO users (wallet_address, chain, balance, is_premium) 
		VALUES ($1, $2, 10, FALSE) 
		ON CONFLICT (wallet_address, chain) DO NOTHING`,
		address, chain,
	)
	if err != nil {
		return nil, err
	}

	// Fetch the created user
	return r.GetUserByWallet(ctx, address, chain)
}

func (r *Repository) UpsertUserNonce(address, chain, nonce string) error {
	ctx := context.Background()
	_, err := r.db.Exec(ctx,
		`INSERT INTO users (wallet_address, chain, nonce, balance) 
		VALUES ($1, $2, $3, 10) 
		ON CONFLICT (wallet_address, chain) 
		DO UPDATE SET nonce = $3, updated_at = NOW()`,
		address, chain, nonce,
	)
	return err
}

func (r *Repository) GetUserNonce(address, chain string) (string, error) {
	ctx := context.Background()
	var nonce string
	err := r.db.QueryRow(ctx,
		`SELECT nonce FROM users WHERE wallet_address = $1 AND chain = $2`,
		address, chain,
	).Scan(&nonce)

	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return nonce, nil
}

func (r *Repository) UpdateLastLogin(userID int64) error {
	ctx := context.Background()
	_, err := r.db.Exec(ctx,
		`UPDATE users SET last_login_at = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

func (r *Repository) AddUserBalance(ctx context.Context, userID int64, checks int) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET balance = balance + $2, updated_at = NOW() WHERE id = $1`,
		userID, checks,
	)
	return err
}

// ==================== Order Methods ====================

func (r *Repository) CreateOrder(ctx context.Context, userID int, checksCount int, totalUSD float64, currency string, tokenAmount float64, paymentAddress string) (*models.Order, error) {
	orderUUID := uuid.New().String()
	
	var orderID int64
	err := r.db.QueryRow(ctx,
		`INSERT INTO orders (user_id, order_uuid, checks_count, total_usd, currency, token_amount, payment_address, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')
		RETURNING id`,
		userID, orderUUID, checksCount, totalUSD, currency, tokenAmount, paymentAddress,
	).Scan(&orderID)

	if err != nil {
		return nil, err
	}

	return &models.Order{
		ID:             orderID,
		UserID:         int64(userID),
		OrderUUID:      orderUUID,
		ChecksCount:    checksCount,
		TotalUSD:       totalUSD,
		Currency:       currency,
		TokenAmount:    tokenAmount,
		PaymentAddress: paymentAddress,
		Status:         "pending",
		CreatedAt:      time.Now(),
	}, nil
}

func (r *Repository) GetOrderByUUID(ctx context.Context, uuid string) (*models.Order, error) {
	var order models.Order
	var completedAt *time.Time
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, order_uuid, checks_count, total_usd, currency, token_amount, payment_address, status, tx_hash, created_at, completed_at
		FROM orders WHERE order_uuid = $1`,
		uuid,
	).Scan(&order.ID, &order.UserID, &order.OrderUUID, &order.ChecksCount, &order.TotalUSD,
		&order.Currency, &order.TokenAmount, &order.PaymentAddress, &order.Status,
		&order.TxHash, &order.CreatedAt, &completedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	order.CompletedAt = completedAt
	return &order, nil
}

func (r *Repository) CompleteOrder(ctx context.Context, orderUUID, txHash string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE orders SET status = 'completed', tx_hash = $2, completed_at = NOW() 
		WHERE order_uuid = $1 AND status = 'pending'`,
		orderUUID, txHash,
	)
	return err
}

func (r *Repository) CancelOrder(ctx context.Context, orderUUID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE orders SET status = 'cancelled' WHERE order_uuid = $1 AND status = 'pending'`,
		orderUUID,
	)
	return err
}

// ==================== Wallet Methods ====================

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

func (r *Repository) GetCheckHistory(ctx context.Context, limit int) ([]models.RecentCheck, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, address, chain, COALESCE(status, 'safe') as status, created_at 
		FROM check_history 
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
