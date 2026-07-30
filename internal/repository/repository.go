package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	"vauln-address/internal/config"
	"vauln-address/internal/models"
)

type Repository struct {
	db             *sql.DB
	dbType         config.DBType
	freeCheckLimit int
}

func New(cfg *config.Config) (*Repository, error) {
	var driverName, dsn string

	switch cfg.DBType {
	case config.DBTypeMySQL:
		driverName = "mysql"
		// Use unix socket if specified, otherwise use TCP
		if cfg.DBUnixSocket != "" {
			dsn = fmt.Sprintf("%s:%s@unix(%s)/%s?charset=%s&parseTime=True",
				cfg.DBUser, cfg.DBPassword, cfg.DBUnixSocket, cfg.DBName, cfg.DBCharset)
		} else {
			dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&tls=false",
				cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBCharset)
		}

	case config.DBTypeSQLite:
		driverName = "sqlite3"
		// Ensure directory exists
		dir := filepath.Dir(cfg.SQLitePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory for SQLite: %w", err)
		}
		dsn = cfg.SQLitePath

	default: // PostgreSQL
		driverName = "postgres"
		dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Repository{db: db, dbType: cfg.DBType, freeCheckLimit: cfg.FreeCheckLimit}, nil
}

func (r *Repository) Close() {
	r.db.Close()
}

// executeStatements splits SQL by semicolons and executes each statement
func (r *Repository) executeStatements(ctx context.Context, schema string) error {
	// Split by semicolon and trim whitespace
	statements := strings.Split(schema, ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			// Ignore common "already exists" errors for CREATE statements
			errMsg := strings.ToLower(err.Error())
			ignoreErrors := []string{
				"already exists",
				"duplicate",
			}
			shouldIgnore := false
			for _, ignore := range ignoreErrors {
				if strings.Contains(errMsg, ignore) {
					shouldIgnore = true
					break
				}
			}
			if !shouldIgnore {
				return fmt.Errorf("failed to execute statement: %w\nStatement: %s", err, stmt)
			}
		}
	}
	return nil
}

func (r *Repository) InitSchema(ctx context.Context) error {
	var schema string

	switch r.dbType {
	case config.DBTypeMySQL:
		schema = `
		CREATE TABLE IF NOT EXISTS wallets (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			address VARCHAR(100) NOT NULL,
			chain VARCHAR(20) NOT NULL,
			status VARCHAR(20) NOT NULL,
			has_pk TINYINT(1) DEFAULT 0,
			has_seed TINYINT(1) DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		);
		CREATE INDEX idx_wallets_address ON wallets(address);
		CREATE INDEX idx_wallets_chain ON wallets(chain);
		CREATE INDEX idx_wallets_status ON wallets(status);
		CREATE INDEX idx_wallets_chain_status ON wallets(chain, status);
		CREATE TABLE IF NOT EXISTS users (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			wallet_address VARCHAR(100) NOT NULL,
			chain VARCHAR(20) NOT NULL,
			nonce VARCHAR(100),
			balance INT DEFAULT 10,
			is_premium TINYINT(1) DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			last_login_at TIMESTAMP NULL,
			UNIQUE KEY uk_users_wallet_chain (wallet_address, chain)
		);
		CREATE INDEX idx_users_wallet ON users(wallet_address);
		CREATE TABLE IF NOT EXISTS orders (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			wallet_address VARCHAR(100) NOT NULL,
			chain VARCHAR(20) NOT NULL,
			order_uuid VARCHAR(100) UNIQUE NOT NULL,
			checks_count INT NOT NULL,
			total_usd DECIMAL(10, 2) NOT NULL,
			currency VARCHAR(20) NOT NULL,
			token_amount DECIMAL(20, 8),
			payment_address VARCHAR(200),
			status VARCHAR(20) DEFAULT 'pending',
			tx_hash VARCHAR(200),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			completed_at TIMESTAMP NULL
		);
		CREATE INDEX idx_orders_wallet ON orders(wallet_address);
		CREATE INDEX idx_orders_uuid ON orders(order_uuid);
		CREATE INDEX idx_orders_status ON orders(status);
		CREATE INDEX idx_orders_wallet_status ON orders(wallet_address, status);
		CREATE TABLE IF NOT EXISTS contact_messages (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) NOT NULL,
			message TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS rate_limits (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			ip_address VARCHAR(45) NOT NULL UNIQUE,
			request_count INT DEFAULT 0,
			window_start TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX idx_rate_limits_ip ON rate_limits(ip_address);
		CREATE TABLE IF NOT EXISTS check_history (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			wallet_address VARCHAR(100) DEFAULT '',
			address VARCHAR(100) NOT NULL,
			chain VARCHAR(20) NOT NULL,
			status VARCHAR(20),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX idx_check_history_created ON check_history(created_at);
		CREATE INDEX idx_check_history_wallet ON check_history(wallet_address);
		CREATE TABLE IF NOT EXISTS api_keys (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			wallet_address VARCHAR(100) NOT NULL,
			key_hash VARCHAR(100) NOT NULL UNIQUE,
			key_prefix VARCHAR(20) NOT NULL,
			name VARCHAR(100) NOT NULL,
			last_used_at TIMESTAMP NULL,
			expires_at TIMESTAMP NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			revoked_at TIMESTAMP NULL,
			is_revoked TINYINT(1) DEFAULT 0
		);
		CREATE INDEX idx_api_keys_wallet ON api_keys(wallet_address);
		CREATE INDEX idx_api_keys_hash ON api_keys(key_hash);
		`

	case config.DBTypeSQLite:
		schema = `
		CREATE TABLE IF NOT EXISTS wallets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			address TEXT NOT NULL,
			chain TEXT NOT NULL,
			status TEXT NOT NULL,
			has_pk INTEGER DEFAULT 0,
			has_seed INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_wallets_address ON wallets(address);
		CREATE INDEX IF NOT EXISTS idx_wallets_chain ON wallets(chain);
		CREATE INDEX IF NOT EXISTS idx_wallets_status ON wallets(status);
		CREATE INDEX IF NOT EXISTS idx_wallets_chain_status ON wallets(chain, status);

		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			wallet_address TEXT NOT NULL,
			chain TEXT NOT NULL,
			nonce TEXT,
			balance INTEGER DEFAULT 10,
			is_premium INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_login_at DATETIME,
			UNIQUE(wallet_address, chain)
		);
		CREATE INDEX IF NOT EXISTS idx_users_wallet ON users(wallet_address);

		CREATE TABLE IF NOT EXISTS orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			wallet_address TEXT NOT NULL,
			chain TEXT NOT NULL,
			order_uuid TEXT UNIQUE NOT NULL,
			checks_count INTEGER NOT NULL,
			total_usd REAL NOT NULL,
			currency TEXT NOT NULL,
			token_amount REAL,
			payment_address TEXT,
			status TEXT DEFAULT 'pending',
			tx_hash TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_orders_wallet ON orders(wallet_address);
		CREATE INDEX IF NOT EXISTS idx_orders_uuid ON orders(order_uuid);
		CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
		CREATE INDEX IF NOT EXISTS idx_orders_wallet_status ON orders(wallet_address, status);

		CREATE TABLE IF NOT EXISTS contact_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS rate_limits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ip_address TEXT NOT NULL UNIQUE,
			request_count INTEGER DEFAULT 0,
			window_start DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_rate_limits_ip ON rate_limits(ip_address);

		CREATE TABLE IF NOT EXISTS check_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			wallet_address TEXT DEFAULT '',
			address TEXT NOT NULL,
			chain TEXT NOT NULL,
			status TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_check_history_created ON check_history(created_at);
		CREATE INDEX IF NOT EXISTS idx_check_history_wallet ON check_history(wallet_address);

		CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			wallet_address TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			key_prefix TEXT NOT NULL,
			name TEXT NOT NULL,
			last_used_at DATETIME,
			expires_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			revoked_at DATETIME,
			is_revoked INTEGER DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_api_keys_wallet ON api_keys(wallet_address);
		CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
		`

	default: // PostgreSQL
		schema = `
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

		CREATE TABLE IF NOT EXISTS orders (
			id BIGSERIAL PRIMARY KEY,
			wallet_address VARCHAR(100) NOT NULL,
			chain VARCHAR(20) NOT NULL,
			order_uuid VARCHAR(100) UNIQUE NOT NULL,
			checks_count INTEGER NOT NULL,
			total_usd DECIMAL(10, 2) NOT NULL,
			currency VARCHAR(20) NOT NULL,
			token_amount DECIMAL(20, 8),
			payment_address VARCHAR(200),
			status VARCHAR(20) DEFAULT 'pending',
			tx_hash VARCHAR(200),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			completed_at TIMESTAMP WITH TIME ZONE
		);
		CREATE INDEX IF NOT EXISTS idx_orders_wallet ON orders(wallet_address);
		CREATE INDEX IF NOT EXISTS idx_orders_uuid ON orders(order_uuid);
		CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
		CREATE INDEX IF NOT EXISTS idx_orders_wallet_status ON orders(wallet_address, status);

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

		CREATE TABLE IF NOT EXISTS check_history (
			id BIGSERIAL PRIMARY KEY,
			wallet_address VARCHAR(100) DEFAULT '',
			address VARCHAR(100) NOT NULL,
			chain VARCHAR(20) NOT NULL,
			status VARCHAR(20),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_check_history_created ON check_history(created_at);
		CREATE INDEX IF NOT EXISTS idx_check_history_wallet ON check_history(wallet_address);

		CREATE TABLE IF NOT EXISTS api_keys (
			id BIGSERIAL PRIMARY KEY,
			wallet_address VARCHAR(100) NOT NULL,
			key_hash VARCHAR(100) NOT NULL UNIQUE,
			key_prefix VARCHAR(20) NOT NULL,
			name VARCHAR(100) NOT NULL,
			last_used_at TIMESTAMP WITH TIME ZONE,
			expires_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			revoked_at TIMESTAMP WITH TIME ZONE,
			is_revoked BOOLEAN DEFAULT FALSE
		);
		CREATE INDEX IF NOT EXISTS idx_api_keys_wallet ON api_keys(wallet_address);
		CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
		`
	}

	if err := r.executeStatements(ctx, schema); err != nil {
		return err
	}

	// Run migrations
	return r.runMigrations(ctx)
}

func (r *Repository) runMigrations(ctx context.Context) error {
	// Migration: add updated_at to orders if not exists
	switch r.dbType {
	case config.DBTypeMySQL:
		_, err := r.db.ExecContext(ctx, "ALTER TABLE orders ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP")
		if err != nil && !strings.Contains(err.Error(), "Duplicate column") {
			log.Printf("Migration note (orders.updated_at): %v", err)
		}
	case config.DBTypeSQLite:
		_, err := r.db.ExecContext(ctx, "ALTER TABLE orders ADD COLUMN updated_at DATETIME DEFAULT CURRENT_TIMESTAMP")
		if err != nil && !strings.Contains(err.Error(), "no such column") && !strings.Contains(err.Error(), "duplicate column") {
			log.Printf("Migration note (orders.updated_at): %v", err)
		}
	default: // PostgreSQL
		_, err := r.db.ExecContext(ctx, "ALTER TABLE orders ADD COLUMN updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()")
		if err != nil && !strings.Contains(err.Error(), "Duplicate column") {
			log.Printf("Migration note (orders.updated_at): %v", err)
		}
	}

	// Migration 004: Create seeds table and add columns to wallets
	r.migration004(ctx)

	return nil
}

func (r *Repository) migration004(ctx context.Context) {
	// Create seeds table
	var createSeedsSQL string
	switch r.dbType {
	case config.DBTypeMySQL:
		createSeedsSQL = `CREATE TABLE IF NOT EXISTS seeds (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			seed_phrase TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`
	case config.DBTypeSQLite:
		createSeedsSQL = `CREATE TABLE IF NOT EXISTS seeds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			seed_phrase TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`
	default: // PostgreSQL
		createSeedsSQL = `CREATE TABLE IF NOT EXISTS seeds (
			id BIGSERIAL PRIMARY KEY,
			seed_phrase TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`
	}
	_, err := r.db.ExecContext(ctx, createSeedsSQL)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		log.Printf("Migration note (seeds table): %v", err)
	}

	// Add columns to wallets table
	var addColumnSQLs []string
	switch r.dbType {
	case config.DBTypeMySQL:
		addColumnSQLs = []string{
			"ALTER TABLE wallets ADD COLUMN seed_id BIGINT DEFAULT 0",
			"ALTER TABLE wallets ADD COLUMN reason VARCHAR(100)",
			"ALTER TABLE wallets ADD COLUMN source VARCHAR(100)",
		}
	case config.DBTypeSQLite:
		addColumnSQLs = []string{
			"ALTER TABLE wallets ADD COLUMN seed_id INTEGER DEFAULT 0",
			"ALTER TABLE wallets ADD COLUMN reason TEXT",
			"ALTER TABLE wallets ADD COLUMN source TEXT",
		}
	default: // PostgreSQL
		addColumnSQLs = []string{
			"ALTER TABLE wallets ADD COLUMN seed_id BIGINT DEFAULT 0",
			"ALTER TABLE wallets ADD COLUMN reason VARCHAR(100)",
			"ALTER TABLE wallets ADD COLUMN source VARCHAR(100)",
		}
	}

	for _, sql := range addColumnSQLs {
		_, err := r.db.ExecContext(ctx, sql)
		if err != nil && !strings.Contains(err.Error(), "Duplicate column") && !strings.Contains(err.Error(), "no such column") {
			log.Printf("Migration note (wallets column): %v", err)
		}
	}
}

// ==================== User Methods ====================

func (r *Repository) GetUserByWallet(ctx context.Context, address, chain string) (*models.User, error) {
	var user models.User
	var lastLoginAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT wallet_address, chain, nonce, balance, created_at, updated_at, last_login_at 
		FROM users WHERE wallet_address = ? AND chain = ?`,
		address, chain,
	).Scan(&user.WalletAddress, &user.Chain, &user.Nonce, &user.Balance,
		&user.CreatedAt, &user.UpdatedAt, &lastLoginAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
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

	// Create new user with free check limit from config
	if r.dbType == config.DBTypeSQLite {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO users (wallet_address, chain, balance, is_premium) 
			VALUES (?, ?, ?, 0)`,
			address, chain, r.freeCheckLimit,
		)
	} else {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO users (wallet_address, chain, balance, is_premium) 
			VALUES (?, ?, ?, FALSE)`,
			address, chain, r.freeCheckLimit,
		)
	}
	if err != nil {
		return nil, err
	}

	// Fetch the created user
	return r.GetUserByWallet(ctx, address, chain)
}

func (r *Repository) UpsertUserNonce(address, chain, nonce string) error {
	ctx := context.Background()

	var err error
	if r.dbType == config.DBTypeSQLite {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO users (wallet_address, chain, nonce, balance) 
			VALUES (?, ?, ?, ?) 
			ON CONFLICT(wallet_address, chain) 
			DO UPDATE SET nonce = ?`,
			address, chain, nonce, r.freeCheckLimit, nonce,
		)
	} else if r.dbType == config.DBTypeMySQL {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO users (wallet_address, chain, nonce, balance) 
			VALUES (?, ?, ?, ?) 
			ON DUPLICATE KEY UPDATE nonce = ?`,
			address, chain, nonce, r.freeCheckLimit, nonce,
		)
	} else {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO users (wallet_address, chain, nonce, balance) 
			VALUES (?, ?, ?, ?) 
			ON CONFLICT (wallet_address, chain) 
			DO UPDATE SET nonce = ?`,
			address, chain, nonce, r.freeCheckLimit, nonce,
		)
	}
	return err
}

func (r *Repository) GetUserNonce(address, chain string) (string, error) {
	ctx := context.Background()
	var nonce sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT nonce FROM users WHERE wallet_address = ? AND chain = ?`,
		address, chain,
	).Scan(&nonce)

	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if nonce.Valid {
		return nonce.String, nil
	}
	return "", nil
}

func (r *Repository) UpdateLastLogin(address, chain string) error {
	ctx := context.Background()
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE wallet_address = ? AND chain = ?`,
		address, chain,
	)
	return err
}

func (r *Repository) AddUserBalance(ctx context.Context, address, chain string, checks int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP WHERE wallet_address = ? AND chain = ?`,
		checks, address, chain,
	)
	return err
}

func (r *Repository) DeductUserBalance(ctx context.Context, address, chain string, checks int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE wallet_address = ? AND chain = ? AND balance >= ?`,
		checks, address, chain, checks,
	)
	return err
}

func (r *Repository) GetUserBalance(ctx context.Context, address, chain string) (int, error) {
	var balance int
	err := r.db.QueryRowContext(ctx,
		`SELECT balance FROM users WHERE wallet_address = ? AND chain = ?`,
		address, chain,
	).Scan(&balance)

	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return balance, nil
}

// GetFreeChecksRemaining returns the number of free checks remaining for today
func (r *Repository) GetFreeChecksRemaining(ctx context.Context, address, chain string) (int, error) {
	// Get free checks used and reset time
	var freeUsed int
	var resetAt time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT free_checks_used, free_checks_reset_at FROM users WHERE wallet_address = ? AND chain = ?`,
		address, chain,
	).Scan(&freeUsed, &resetAt)

	if err == sql.ErrNoRows {
		return r.freeCheckLimit, nil
	}
	if err != nil {
		return 0, err
	}

	// Check if we need to reset (past midnight UTC)
	midnight := nextMidnightUTC()
	if resetAt.Before(midnight) || resetAt.Equal(midnight) {
		// Reset free checks
		_, err = r.db.ExecContext(ctx,
			`UPDATE users SET free_checks_used = 0, free_checks_reset_at = ? WHERE wallet_address = ? AND chain = ?`,
			time.Now().UTC(), address, chain,
		)
		if err != nil {
			return 0, err
		}
		return r.freeCheckLimit, nil
	}

	remaining := r.freeCheckLimit - freeUsed
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}

// DeductFreeCheck decrements the free check counter for today
func (r *Repository) DeductFreeCheck(ctx context.Context, address, chain string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET free_checks_used = free_checks_used + 1 WHERE wallet_address = ? AND chain = ?`,
		address, chain,
	)
	return err
}

// nextMidnightUTC returns the next midnight (00:00 UTC)
func nextMidnightUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
}

// ==================== Order Methods ====================

func (r *Repository) CreateOrder(ctx context.Context, walletAddress, chain string, checksCount int, totalUSD float64, currency string, tokenAmount float64, paymentAddress string) (*models.Order, error) {
	orderUUID := uuid.New().String()

	var result sql.Result
	var err error

	if r.dbType == config.DBTypeSQLite {
		result, err = r.db.ExecContext(ctx,
			`INSERT INTO orders (wallet_address, chain, order_uuid, checks_count, total_usd, currency, token_amount, payment_address, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
			walletAddress, chain, orderUUID, checksCount, totalUSD, currency, tokenAmount, paymentAddress,
		)
	} else {
		result, err = r.db.ExecContext(ctx,
			`INSERT INTO orders (wallet_address, chain, order_uuid, checks_count, total_usd, currency, token_amount, payment_address, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
			walletAddress, chain, orderUUID, checksCount, totalUSD, currency, tokenAmount, paymentAddress,
		)
	}
	if err != nil {
		return nil, err
	}

	_, err = result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &models.Order{
		OrderUUID:      orderUUID,
		WalletAddress:  walletAddress,
		Chain:          chain,
		ChecksCount:    checksCount,
		TotalUSD:       totalUSD,
		Currency:       currency,
		TokenAmount:    tokenAmount,
		PaymentAddress: paymentAddress,
		Status:         "pending",
		CreatedAt:      time.Now(),
	}, nil
}

func (r *Repository) GetOrderByUUID(ctx context.Context, orderUUID string) (*models.Order, error) {
	var order models.Order
	var completedAt sql.NullTime
	var txHash sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT order_uuid, wallet_address, chain, checks_count, total_usd, currency, token_amount, payment_address, status, tx_hash, created_at, completed_at
		FROM orders WHERE order_uuid = ?`,
		orderUUID,
	).Scan(&order.OrderUUID, &order.WalletAddress, &order.Chain, &order.ChecksCount, &order.TotalUSD,
		&order.Currency, &order.TokenAmount, &order.PaymentAddress, &order.Status,
		&txHash, &order.CreatedAt, &completedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if txHash.Valid {
		order.TxHash = txHash.String
	}
	if completedAt.Valid {
		order.CompletedAt = &completedAt.Time
	}
	return &order, nil
}

func (r *Repository) CompleteOrder(ctx context.Context, orderUUID, txHash string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE orders SET status = 'completed', tx_hash = ?, completed_at = CURRENT_TIMESTAMP 
		WHERE order_uuid = ? AND status = 'pending'`,
		txHash, orderUUID,
	)
	return err
}

func (r *Repository) CancelOrder(ctx context.Context, orderUUID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE orders SET status = 'cancelled' WHERE order_uuid = ? AND status = 'pending'`,
		orderUUID,
	)
	return err
}

// GetOrderByTxHash finds an order by its transaction hash
func (r *Repository) GetOrderByTxHash(ctx context.Context, txHash string) (*models.Order, error) {
	var order models.Order
	var txHashNull sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT order_uuid, wallet_address, chain, status, checks_count, total_usd, currency, token_amount, payment_address, tx_hash, created_at, updated_at
		FROM orders WHERE tx_hash = ? LIMIT 1`,
		txHash,
	).Scan(&order.OrderUUID, &order.WalletAddress, &order.Chain, &order.Status, &order.ChecksCount, &order.TotalUSD, &order.Currency, &order.TokenAmount, &order.PaymentAddress, &txHashNull, &order.CreatedAt, &order.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if txHashNull.Valid {
		order.TxHash = txHashNull.String
	}
	return &order, nil
}

// GetPendingOrderByWallet finds a pending order for a wallet
func (r *Repository) GetPendingOrderByWallet(ctx context.Context, walletAddress string) (*models.Order, error) {
	var order models.Order
	var txHashNull sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT order_uuid, wallet_address, chain, status, checks_count, total_usd, currency, token_amount, payment_address, tx_hash, created_at, updated_at
		FROM orders WHERE wallet_address = ? AND status = 'pending' ORDER BY created_at DESC LIMIT 1`,
		walletAddress,
	).Scan(&order.OrderUUID, &order.WalletAddress, &order.Chain, &order.Status, &order.ChecksCount, &order.TotalUSD, &order.Currency, &order.TokenAmount, &order.PaymentAddress, &txHashNull, &order.CreatedAt, &order.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if txHashNull.Valid {
		order.TxHash = txHashNull.String
	}
	return &order, nil
}

// GetOrdersByWallet retrieves all orders for a wallet address
func (r *Repository) GetOrdersByWallet(ctx context.Context, walletAddress string, limit int) ([]models.Order, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT order_uuid, wallet_address, chain, checks_count, total_usd, currency, token_amount, payment_address, status, tx_hash, created_at, completed_at
		FROM orders WHERE wallet_address = ? ORDER BY created_at DESC LIMIT ?`,
		walletAddress, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var order models.Order
		var txHashNull sql.NullString
		var completedAt sql.NullTime

		if err := rows.Scan(&order.OrderUUID, &order.WalletAddress, &order.Chain, &order.ChecksCount, &order.TotalUSD,
			&order.Currency, &order.TokenAmount, &order.PaymentAddress, &order.Status,
			&txHashNull, &order.CreatedAt, &completedAt); err != nil {
			return nil, err
		}

		if txHashNull.Valid {
			order.TxHash = txHashNull.String
		}
		if completedAt.Valid {
			order.CompletedAt = &completedAt.Time
		}
		orders = append(orders, order)
	}
	return orders, rows.Err()
}

// GetOrdersByWalletPaginated retrieves orders with pagination
func (r *Repository) GetOrdersByWalletPaginated(ctx context.Context, walletAddress string, limit, offset int) ([]models.Order, int, error) {
	// Get total count
	var total int
	countRow := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM orders WHERE wallet_address = ?`,
		walletAddress,
	)
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get paginated orders
	rows, err := r.db.QueryContext(ctx,
		`SELECT order_uuid, wallet_address, chain, checks_count, total_usd, currency, token_amount, payment_address, status, tx_hash, created_at, completed_at
			FROM orders WHERE wallet_address = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		walletAddress, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var order models.Order
		var txHashNull sql.NullString
		var completedAt sql.NullTime

		if err := rows.Scan(&order.OrderUUID, &order.WalletAddress, &order.Chain, &order.ChecksCount, &order.TotalUSD,
			&order.Currency, &order.TokenAmount, &order.PaymentAddress, &order.Status,
			&txHashNull, &order.CreatedAt, &completedAt); err != nil {
			return nil, 0, err
		}

		if txHashNull.Valid {
			order.TxHash = txHashNull.String
		}
		if completedAt.Valid {
			order.CompletedAt = &completedAt.Time
		}
		orders = append(orders, order)
	}
	return orders, total, rows.Err()
}

// ==================== Wallet Methods ====================

func (r *Repository) GetWallet(ctx context.Context, address string, chain string) (*models.Wallet, error) {
	var wallet models.Wallet
	err := r.db.QueryRowContext(ctx,
		`SELECT id, address, chain, status, has_pk, has_seed, created_at, updated_at 
		FROM wallets 
		WHERE address = ? AND chain = ?`,
		address, chain,
	).Scan(&wallet.ID, &wallet.Address, &wallet.Chain, &wallet.Status,
		&wallet.HasPK, &wallet.HasSeed, &wallet.CreatedAt, &wallet.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &wallet, nil
}

func (r *Repository) SaveContactMessage(ctx context.Context, msg *models.ContactMessage) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO contact_messages (name, email, message) VALUES (?, ?, ?)`,
		msg.Name, msg.Email, msg.Message,
	)
	return err
}

func (r *Repository) GetRecentChecks(ctx context.Context, limit int) ([]models.RecentCheck, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, address, chain, status, created_at 
		FROM wallets 
		ORDER BY created_at DESC 
		LIMIT ?`,
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
	err := r.db.QueryRowContext(ctx,
		`SELECT id, ip_address, request_count, window_start 
		FROM rate_limits 
		WHERE ip_address = ?`,
		ip,
	).Scan(&rl.ID, &rl.IPAddress, &rl.Count, &rl.WindowStart)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rl, nil
}

func (r *Repository) IncrementRateLimit(ctx context.Context, ip string, windowStart time.Time) error {
	var err error
	if r.dbType == config.DBTypeSQLite {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO rate_limits (ip_address, request_count, window_start) 
			VALUES (?, 1, ?) 
			ON CONFLICT(ip_address) 
			DO UPDATE SET 
				request_count = rate_limits.request_count + 1,
				window_start = CASE 
					WHEN rate_limits.window_start < ? THEN ? 
					ELSE rate_limits.window_start 
				END`,
			ip, windowStart, windowStart, windowStart,
		)
	} else if r.dbType == config.DBTypeMySQL {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO rate_limits (ip_address, request_count, window_start) 
			VALUES (?, 1, ?) 
			ON DUPLICATE KEY UPDATE 
				request_count = request_count + 1,
				window_start = CASE 
					WHEN window_start < ? THEN ? 
					ELSE window_start 
				END`,
			ip, windowStart, windowStart, windowStart,
		)
	} else {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO rate_limits (ip_address, request_count, window_start) 
			VALUES (?, 1, ?) 
			ON CONFLICT (ip_address) 
			DO UPDATE SET 
				request_count = rate_limits.request_count + 1,
				window_start = CASE 
					WHEN rate_limits.window_start < ? THEN ? 
					ELSE rate_limits.window_start 
				END`,
			ip, windowStart, windowStart, windowStart,
		)
	}
	return err
}

func (r *Repository) ResetRateLimit(ctx context.Context, ip string, windowStart time.Time) error {
	var err error
	if r.dbType == config.DBTypeSQLite {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO rate_limits (ip_address, request_count, window_start) 
			VALUES (?, 0, ?) 
			ON CONFLICT(ip_address) 
			DO UPDATE SET request_count = 0, window_start = ?`,
			ip, windowStart, windowStart,
		)
	} else if r.dbType == config.DBTypeMySQL {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO rate_limits (ip_address, request_count, window_start) 
			VALUES (?, 0, ?) 
			ON DUPLICATE KEY UPDATE request_count = 0, window_start = ?`,
			ip, windowStart, windowStart,
		)
	} else {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO rate_limits (ip_address, request_count, window_start) 
			VALUES (?, 0, ?) 
			ON CONFLICT (ip_address) 
			DO UPDATE SET request_count = 0, window_start = ?`,
			ip, windowStart, windowStart,
		)
	}
	return err
}

// ResetAllRateLimits resets rate limits for all IPs (used for daily reset at 00:00)
func (r *Repository) ResetAllRateLimits(ctx context.Context) (int64, error) {
	var err error
	var result sql.Result
	if r.dbType == config.DBTypeMySQL {
		result, err = r.db.ExecContext(ctx, `UPDATE rate_limits SET request_count = 0, window_start = NOW()`)
	} else {
		result, err = r.db.ExecContext(ctx, `UPDATE rate_limits SET request_count = 0, window_start = NOW()`)
	}
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *Repository) RecordCheck(ctx context.Context, walletAddress, address, chain, status string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO check_history (wallet_address, address, chain, status) VALUES (?, ?, ?, ?)`,
		walletAddress, address, chain, status,
	)
	return err
}

func (r *Repository) GetCheckHistory(ctx context.Context, limit int) ([]models.RecentCheck, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, address, chain, COALESCE(status, 'safe'), created_at 
		FROM check_history 
		ORDER BY created_at DESC 
		LIMIT ?`,
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

// GetCheckHistoryByWallet returns check history for a specific wallet address with pagination
func (r *Repository) GetCheckHistoryByWallet(ctx context.Context, walletAddress string, limit, offset int) ([]models.RecentCheck, int, error) {
	// Count total for this wallet
	var total int
	countQuery := `SELECT COUNT(*) FROM check_history WHERE wallet_address = ?`
	err := r.db.QueryRowContext(ctx, countQuery, walletAddress).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, address, chain, COALESCE(status, 'safe'), created_at 
		FROM check_history 
		WHERE wallet_address = ?
		ORDER BY created_at DESC 
		LIMIT ? OFFSET ?`,
		walletAddress, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var checks []models.RecentCheck
	for rows.Next() {
		var check models.RecentCheck
		if err := rows.Scan(&check.ID, &check.Address, &check.Chain, &check.Status, &check.CheckedAt); err != nil {
			return nil, 0, err
		}
		checks = append(checks, check)
	}
	return checks, total, rows.Err()
}

// ==================== API Key Methods ====================

func (r *Repository) CreateAPIKey(ctx context.Context, walletAddress, keyHash, keyPrefix, name string, expiresAt *time.Time) (*models.APIKey, error) {
	var result sql.Result
	var err error

	if r.dbType == config.DBTypeSQLite {
		result, err = r.db.ExecContext(ctx,
			`INSERT INTO api_keys (wallet_address, key_hash, key_prefix, name, expires_at, created_at, is_revoked)
			VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, 0)`,
			walletAddress, keyHash, keyPrefix, name, expiresAt,
		)
	} else {
		result, err = r.db.ExecContext(ctx,
			`INSERT INTO api_keys (wallet_address, key_hash, key_prefix, name, expires_at, created_at, is_revoked)
			VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, false)`,
			walletAddress, keyHash, keyPrefix, name, expiresAt,
		)
	}
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &models.APIKey{
		ID:            id,
		WalletAddress: walletAddress,
		KeyHash:      keyHash,
		KeyPrefix:    keyPrefix,
		Name:         name,
		ExpiresAt:    expiresAt,
		CreatedAt:    time.Now(),
		IsRevoked:    false,
	}, nil
}

func (r *Repository) GetAPIKeyByID(ctx context.Context, keyID int64) (*models.APIKey, error) {
	var key models.APIKey
	var expiresAt, lastUsedAt, revokedAt sql.NullTime

	err := r.db.QueryRowContext(ctx,
		`SELECT id, wallet_address, key_hash, key_prefix, name, last_used_at, expires_at, created_at, revoked_at, is_revoked
		FROM api_keys WHERE id = ?`,
		keyID,
	).Scan(&key.ID, &key.WalletAddress, &key.KeyHash, &key.KeyPrefix, &key.Name, &lastUsedAt, &expiresAt, &key.CreatedAt, &revokedAt, &key.IsRevoked)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if expiresAt.Valid {
		key.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		key.LastUsedAt = &lastUsedAt.Time
	}
	if revokedAt.Valid {
		key.RevokedAt = &revokedAt.Time
	}

	return &key, nil
}

func (r *Repository) GetAPIKeyByHash(ctx context.Context, keyHash string) (*models.APIKey, error) {
	var key models.APIKey
	var expiresAt, lastUsedAt, revokedAt sql.NullTime

	err := r.db.QueryRowContext(ctx,
		`SELECT id, wallet_address, key_hash, key_prefix, name, last_used_at, expires_at, created_at, revoked_at, is_revoked
		FROM api_keys WHERE key_hash = ?`,
		keyHash,
	).Scan(&key.ID, &key.WalletAddress, &key.KeyHash, &key.KeyPrefix, &key.Name, &lastUsedAt, &expiresAt, &key.CreatedAt, &revokedAt, &key.IsRevoked)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if expiresAt.Valid {
		key.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		key.LastUsedAt = &lastUsedAt.Time
	}
	if revokedAt.Valid {
		key.RevokedAt = &revokedAt.Time
	}

	return &key, nil
}

func (r *Repository) GetUserAPIKeys(ctx context.Context, walletAddress string) ([]models.APIKey, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, wallet_address, key_hash, key_prefix, name, last_used_at, expires_at, created_at, revoked_at, is_revoked
		FROM api_keys WHERE wallet_address = ? ORDER BY created_at DESC`,
		walletAddress,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var key models.APIKey
		var expiresAt, lastUsedAt, revokedAt sql.NullTime

		if err := rows.Scan(&key.ID, &key.WalletAddress, &key.KeyHash, &key.KeyPrefix, &key.Name, &lastUsedAt, &expiresAt, &key.CreatedAt, &revokedAt, &key.IsRevoked); err != nil {
			return nil, err
		}

		if expiresAt.Valid {
			key.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			key.LastUsedAt = &lastUsedAt.Time
		}
		if revokedAt.Valid {
			key.RevokedAt = &revokedAt.Time
		}

		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (r *Repository) RevokeAPIKey(ctx context.Context, keyID int64, walletAddress string) error {
	var err error
	if r.dbType == config.DBTypeSQLite {
		_, err = r.db.ExecContext(ctx,
			`UPDATE api_keys SET is_revoked = 1, revoked_at = CURRENT_TIMESTAMP WHERE id = ? AND wallet_address = ?`,
			keyID, walletAddress,
		)
	} else {
		_, err = r.db.ExecContext(ctx,
			`UPDATE api_keys SET is_revoked = true, revoked_at = CURRENT_TIMESTAMP WHERE id = ? AND wallet_address = ?`,
			keyID, walletAddress,
		)
	}
	return err
}

func (r *Repository) UpdateAPIKeyLastUsed(ctx context.Context, keyID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`,
		keyID,
	)
	return err
}

func (r *Repository) DeleteAPIKey(ctx context.Context, keyID int64, walletAddress string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM api_keys WHERE id = ? AND wallet_address = ?`,
		keyID, walletAddress,
	)
	return err
}

// ==================== Admin Methods ====================

// GetSeedByPhrase checks if a seed phrase exists and returns its ID
func (r *Repository) GetSeedByPhrase(ctx context.Context, seedPhrase string) (int64, bool, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM seeds WHERE seed_phrase = ?`,
		seedPhrase,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// SaveSeed saves a seed phrase and returns the ID (only if not exists)
func (r *Repository) SaveSeed(ctx context.Context, seedPhrase string) (int64, bool, error) {
	// First check if exists
	id, exists, err := r.GetSeedByPhrase(ctx, seedPhrase)
	if err != nil {
		return 0, false, err
	}
	if exists {
		return id, false, nil
	}

	// Insert new
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO seeds (seed_phrase) VALUES (?)`,
		seedPhrase,
	)
	if err != nil {
		return 0, false, err
	}
	newID, _ := result.LastInsertId()
	return newID, true, nil
}

// GetWalletByAddressAndChain checks if a wallet exists for address+chain and returns its ID
func (r *Repository) GetWalletByAddressAndChain(ctx context.Context, address, chain string) (int64, bool, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM wallets WHERE address = ? AND chain = ?`,
		address, chain,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// CreateWalletWithSeed creates a wallet with a reference to a seed
func (r *Repository) CreateWalletWithSeed(ctx context.Context, address, chain string, status models.WalletStatus, seedID int64, reason, source string) (int64, error) {
	var hasSeed bool
	if seedID > 0 {
		hasSeed = true
	}

	now := time.Now()
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO wallets (address, chain, status, has_pk, has_seed, seed_id, reason, source, created_at, updated_at) 
		VALUES (?, ?, ?, false, ?, ?, ?, ?, ?, ?)`,
		address, chain, status, hasSeed, seedID, reason, source, now, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// CreateWallet creates a wallet without seed reference
func (r *Repository) CreateWallet(ctx context.Context, address, chain string, status models.WalletStatus, reason, source string) (int64, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO wallets (address, chain, status, has_pk, has_seed, reason, source, created_at, updated_at) 
		VALUES (?, ?, ?, false, false, ?, ?, ?, ?)`,
		address, chain, status, reason, source, now, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateWalletSeed updates a wallet's seed reference
func (r *Repository) UpdateWalletSeed(ctx context.Context, walletID int64, seedID int64) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		`UPDATE wallets SET has_seed = true, seed_id = ?, updated_at = ? WHERE id = ?`,
		seedID, now, walletID,
	)
	return err
}

// GetExpiredOrders returns pending orders older than the specified duration
func (r *Repository) GetExpiredOrders(ctx context.Context, olderThan time.Duration) ([]models.Order, error) {
	cutoff := time.Now().Add(-olderThan)
	rows, err := r.db.QueryContext(ctx,
		`SELECT order_uuid, wallet_address, chain, status, checks_count, total_usd, currency, token_amount, payment_address, tx_hash, created_at, updated_at
		FROM orders 
		WHERE status = 'pending' AND created_at < ?
		ORDER BY created_at ASC`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var order models.Order
		var txHashNull sql.NullString
		if err := rows.Scan(&order.OrderUUID, &order.WalletAddress, &order.Chain, &order.Status, &order.ChecksCount, &order.TotalUSD, &order.Currency, &order.TokenAmount, &order.PaymentAddress, &txHashNull, &order.CreatedAt, &order.UpdatedAt); err != nil {
			return nil, err
		}
		if txHashNull.Valid {
			order.TxHash = txHashNull.String
		}
		orders = append(orders, order)
	}
	return orders, rows.Err()
}

// ExpireOrders marks pending orders as expired if they're older than the duration
func (r *Repository) ExpireOrders(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := r.db.ExecContext(ctx,
		`UPDATE orders SET status = 'expired' WHERE status = 'pending' AND created_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Stats table management for wallet counts

// InitStatsTable creates the stats table if not exists
func (r *Repository) InitStatsTable(ctx context.Context) error {
	var sql string
	switch r.dbType {
	case config.DBTypeMySQL:
		sql = `CREATE TABLE IF NOT EXISTS wallet_stats (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			chain VARCHAR(20) UNIQUE NOT NULL,
			count INTEGER DEFAULT 0
		)`
	case config.DBTypeSQLite:
		sql = `CREATE TABLE IF NOT EXISTS wallet_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chain TEXT UNIQUE NOT NULL,
			count INTEGER DEFAULT 0
		)`
	default: // PostgreSQL
		sql = `CREATE TABLE IF NOT EXISTS wallet_stats (
			id BIGSERIAL PRIMARY KEY,
			chain VARCHAR(20) UNIQUE NOT NULL,
			count INTEGER DEFAULT 0
		)`
	}

	_, err := r.db.ExecContext(ctx, sql)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}

	// Initialize counts from wallets table if stats are empty
	var initSQL string
	if r.dbType == config.DBTypeSQLite {
		initSQL = `INSERT OR IGNORE INTO wallet_stats (chain, count) 
			SELECT chain, COUNT(*) FROM wallets GROUP BY chain`
	} else {
		// MySQL syntax
		initSQL = `INSERT IGNORE INTO wallet_stats (chain, count) 
			SELECT chain, COUNT(*) FROM wallets GROUP BY chain`
	}
	_, err = r.db.ExecContext(ctx, initSQL)
	return err
}

// IncrementWalletCount increments the wallet count for a chain
func (r *Repository) IncrementWalletCount(ctx context.Context, chain string) error {
	// First ensure the row exists
	var insertSQL string
	if r.dbType == config.DBTypeSQLite {
		insertSQL = `INSERT OR IGNORE INTO wallet_stats (chain, count) VALUES (?, 0)`
	} else {
		insertSQL = `INSERT IGNORE INTO wallet_stats (chain, count) VALUES (?, 0)`
	}
	_, err := r.db.ExecContext(ctx, insertSQL, chain)
	if err != nil {
		return err
	}

	// Then increment
	_, err = r.db.ExecContext(ctx,
		`UPDATE wallet_stats SET count = count + 1 WHERE chain = ?`, chain)
	return err
}

// GetWalletStats returns the count of wallets per chain from stats table
func (r *Repository) GetWalletStats(ctx context.Context) (map[string]int, error) {
	stats := make(map[string]int)

	rows, err := r.db.QueryContext(ctx, `SELECT chain, count FROM wallet_stats`)
	if err != nil {
		// Fallback to counting from wallets table
		return r.getWalletStatsFromWallets(ctx)
	}
	defer rows.Close()

	for rows.Next() {
		var chain string
		var count int
		if err := rows.Scan(&chain, &count); err != nil {
			continue
		}
		stats[chain] = count
	}

	return stats, rows.Err()
}

// getWalletStatsFromWallets counts wallets directly from wallets table
func (r *Repository) getWalletStatsFromWallets(ctx context.Context) (map[string]int, error) {
	stats := make(map[string]int)

	rows, err := r.db.QueryContext(ctx, `SELECT chain, COUNT(*) FROM wallets GROUP BY chain`)
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	for rows.Next() {
		var chain string
		var count int
		if err := rows.Scan(&chain, &count); err != nil {
			continue
		}
		stats[chain] = count
	}

	return stats, rows.Err()
}
