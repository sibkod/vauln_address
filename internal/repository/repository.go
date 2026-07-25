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
	db     *sql.DB
	dbType config.DBType
}

func New(cfg *config.Config) (*Repository, error) {
	var driverName, dsn string

	switch cfg.DBType {
	case config.DBTypeMySQL:
		driverName = "mysql"
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True",
			cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBCharset)

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

	return &Repository{db: db, dbType: cfg.DBType}, nil
}

func (r *Repository) Close() {
	r.db.Close()
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
			user_id BIGINT,
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
			completed_at TIMESTAMP NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		);
		CREATE INDEX idx_orders_user ON orders(user_id);
		CREATE INDEX idx_orders_uuid ON orders(order_uuid);
		CREATE INDEX idx_orders_status ON orders(status);

		-- Migration: add updated_at to orders (ignore error if column exists)
		-- We'll handle this in code after table creation

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
			address VARCHAR(100) NOT NULL,
			chain VARCHAR(20) NOT NULL,
			status VARCHAR(20),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX idx_check_history_created ON check_history(created_at);

		CREATE TABLE IF NOT EXISTS api_keys (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			key_hash VARCHAR(100) NOT NULL UNIQUE,
			key_prefix VARCHAR(20) NOT NULL,
			name VARCHAR(100) NOT NULL,
			last_used_at TIMESTAMP NULL,
			expires_at TIMESTAMP NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			revoked_at TIMESTAMP NULL,
			is_revoked TINYINT(1) DEFAULT 0,
			FOREIGN KEY (user_id) REFERENCES users(id)
		);
		CREATE INDEX idx_api_keys_user ON api_keys(user_id);
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
			user_id INTEGER,
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
			completed_at DATETIME,
			FOREIGN KEY (user_id) REFERENCES users(id)
		);
		CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(user_id);
		CREATE INDEX IF NOT EXISTS idx_orders_uuid ON orders(order_uuid);
		CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);

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
			address TEXT NOT NULL,
			chain TEXT NOT NULL,
			status TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_check_history_created ON check_history(created_at);

		CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			key_prefix TEXT NOT NULL,
			name TEXT NOT NULL,
			last_used_at DATETIME,
			expires_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			revoked_at DATETIME,
			is_revoked INTEGER DEFAULT 0,
			FOREIGN KEY (user_id) REFERENCES users(id)
		);
		CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);
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
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			completed_at TIMESTAMP WITH TIME ZONE
		);
		CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(user_id);
		CREATE INDEX IF NOT EXISTS idx_orders_uuid ON orders(order_uuid);
		CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);

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
			address VARCHAR(100) NOT NULL,
			chain VARCHAR(20) NOT NULL,
			status VARCHAR(20),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_check_history_created ON check_history(created_at);

		CREATE TABLE IF NOT EXISTS api_keys (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL,
			key_hash VARCHAR(100) NOT NULL UNIQUE,
			key_prefix VARCHAR(20) NOT NULL,
			name VARCHAR(100) NOT NULL,
			last_used_at TIMESTAMP WITH TIME ZONE,
			expires_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			revoked_at TIMESTAMP WITH TIME ZONE,
			is_revoked BOOLEAN DEFAULT FALSE,
			FOREIGN KEY (user_id) REFERENCES users(id)
		);
		CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);
		CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
		`
	}

	_, err := r.db.ExecContext(ctx, schema)
	if err != nil {
		return err
	}

	// Run migrations
	return r.runMigrations(ctx)
}

func (r *Repository) runMigrations(ctx context.Context) error {
	// Migration: add updated_at to orders if not exists
	// For MySQL/MariaDB
	_, err := r.db.ExecContext(ctx, "ALTER TABLE orders ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP")
	if err != nil && !strings.Contains(err.Error(), "Duplicate column") {
		log.Printf("Migration note: %v (this is ok if column already exists)", err)
	}
	// For SQLite
	_, err = r.db.ExecContext(ctx, "ALTER TABLE orders ADD COLUMN updated_at DATETIME DEFAULT CURRENT_TIMESTAMP")
	if err != nil && !strings.Contains(err.Error(), "no such column") && !strings.Contains(err.Error(), "duplicate column") {
		log.Printf("Migration note: %v (this is ok if column already exists)", err)
	}
	return nil
}

// ==================== User Methods ====================

func (r *Repository) GetUserByWallet(ctx context.Context, address, chain string) (*models.User, error) {
	var user models.User
	var lastLoginAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT id, wallet_address, chain, nonce, balance, created_at, updated_at, last_login_at 
		FROM users WHERE wallet_address = ? AND chain = ?`,
		address, chain,
	).Scan(&user.ID, &user.WalletAddress, &user.Chain, &user.Nonce, &user.Balance,
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

func (r *Repository) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	var user models.User
	var lastLoginAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT id, wallet_address, chain, nonce, balance, created_at, updated_at, last_login_at 
		FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &user.WalletAddress, &user.Chain, &user.Nonce, &user.Balance,
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

	// Create new user with 10 free checks
	if r.dbType == config.DBTypeSQLite {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO users (wallet_address, chain, balance, is_premium) 
			VALUES (?, ?, 10, 0)`,
			address, chain,
		)
	} else {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO users (wallet_address, chain, balance, is_premium) 
			VALUES (?, ?, 10, FALSE)`,
			address, chain,
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
			VALUES (?, ?, ?, 10) 
			ON CONFLICT(wallet_address, chain) 
			DO UPDATE SET nonce = ?`,
			address, chain, nonce, nonce,
		)
	} else {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO users (wallet_address, chain, nonce, balance) 
			VALUES (?, ?, ?, 10) 
			ON CONFLICT (wallet_address, chain) 
			DO UPDATE SET nonce = ?`,
			address, chain, nonce, nonce,
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

func (r *Repository) UpdateLastLogin(userID int64) error {
	ctx := context.Background()
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?`,
		userID,
	)
	return err
}

func (r *Repository) AddUserBalance(ctx context.Context, userID int64, checks int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		checks, userID,
	)
	return err
}

func (r *Repository) DeductUserBalance(ctx context.Context, userID int64, checks int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND balance >= ?`,
		checks, userID, checks,
	)
	return err
}

// ==================== Order Methods ====================

func (r *Repository) CreateOrder(ctx context.Context, userID int, checksCount int, totalUSD float64, currency string, tokenAmount float64, paymentAddress string) (*models.Order, error) {
	orderUUID := uuid.New().String()

	var result sql.Result
	var err error

	if r.dbType == config.DBTypeSQLite {
		result, err = r.db.ExecContext(ctx,
			`INSERT INTO orders (user_id, order_uuid, checks_count, total_usd, currency, token_amount, payment_address, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')`,
			userID, orderUUID, checksCount, totalUSD, currency, tokenAmount, paymentAddress,
		)
	} else {
		result, err = r.db.ExecContext(ctx,
			`INSERT INTO orders (user_id, order_uuid, checks_count, total_usd, currency, token_amount, payment_address, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')`,
			userID, orderUUID, checksCount, totalUSD, currency, tokenAmount, paymentAddress,
		)
	}
	if err != nil {
		return nil, err
	}

	orderID, err := result.LastInsertId()
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

func (r *Repository) GetOrderByUUID(ctx context.Context, orderUUID string) (*models.Order, error) {
	var order models.Order
	var completedAt sql.NullTime
	var txHash sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, order_uuid, checks_count, total_usd, currency, token_amount, payment_address, status, tx_hash, created_at, completed_at
		FROM orders WHERE order_uuid = ?`,
		orderUUID,
	).Scan(&order.ID, &order.UserID, &order.OrderUUID, &order.ChecksCount, &order.TotalUSD,
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
		`SELECT id, user_id, order_uuid, status, checks_count, total_usd, currency, token_amount, payment_address, tx_hash, created_at, updated_at
		FROM orders WHERE tx_hash = ? LIMIT 1`,
		txHash,
	).Scan(&order.ID, &order.UserID, &order.OrderUUID, &order.Status, &order.ChecksCount, &order.TotalUSD, &order.Currency, &order.TokenAmount, &order.PaymentAddress, &txHashNull, &order.CreatedAt, &order.UpdatedAt)

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

// GetPendingOrderByUser finds a pending order for a user
func (r *Repository) GetPendingOrderByUser(ctx context.Context, userID int64) (*models.Order, error) {
	var order models.Order
	var txHashNull sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, order_uuid, status, checks_count, total_usd, currency, token_amount, payment_address, tx_hash, created_at, updated_at
		FROM orders WHERE user_id = ? AND status = 'pending' ORDER BY created_at DESC LIMIT 1`,
		userID,
	).Scan(&order.ID, &order.UserID, &order.OrderUUID, &order.Status, &order.ChecksCount, &order.TotalUSD, &order.Currency, &order.TokenAmount, &order.PaymentAddress, &txHashNull, &order.CreatedAt, &order.UpdatedAt)

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
			VALUES (?, 1, ?) 
			ON CONFLICT(ip_address) 
			DO UPDATE SET request_count = 1, window_start = ?`,
			ip, windowStart, windowStart,
		)
	} else {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO rate_limits (ip_address, request_count, window_start) 
			VALUES (?, 1, ?) 
			ON CONFLICT (ip_address) 
			DO UPDATE SET request_count = 1, window_start = ?`,
			ip, windowStart, windowStart,
		)
	}
	return err
}

func (r *Repository) RecordCheck(ctx context.Context, address, chain, status string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO check_history (address, chain, status) VALUES (?, ?, ?)`,
		address, chain, status,
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

// ==================== API Key Methods ====================

func (r *Repository) CreateAPIKey(ctx context.Context, userID int64, keyHash, keyPrefix, name string, expiresAt *time.Time) (*models.APIKey, error) {
	var result sql.Result
	var err error

	if r.dbType == config.DBTypeSQLite {
		result, err = r.db.ExecContext(ctx,
			`INSERT INTO api_keys (user_id, key_hash, key_prefix, name, expires_at, created_at, is_revoked)
			VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, 0)`,
			userID, keyHash, keyPrefix, name, expiresAt,
		)
	} else {
		result, err = r.db.ExecContext(ctx,
			`INSERT INTO api_keys (user_id, key_hash, key_prefix, name, expires_at, created_at, is_revoked)
			VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, false)`,
			userID, keyHash, keyPrefix, name, expiresAt,
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
		ID:        id,
		UserID:    userID,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		Name:      name,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
		IsRevoked: false,
	}, nil
}

func (r *Repository) GetAPIKeyByID(ctx context.Context, keyID int64) (*models.APIKey, error) {
	var key models.APIKey
	var expiresAt, lastUsedAt, revokedAt sql.NullTime

	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, key_hash, key_prefix, name, last_used_at, expires_at, created_at, revoked_at, is_revoked
		FROM api_keys WHERE id = ?`,
		keyID,
	).Scan(&key.ID, &key.UserID, &key.KeyHash, &key.KeyPrefix, &key.Name, &lastUsedAt, &expiresAt, &key.CreatedAt, &revokedAt, &key.IsRevoked)

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
		`SELECT id, user_id, key_hash, key_prefix, name, last_used_at, expires_at, created_at, revoked_at, is_revoked
		FROM api_keys WHERE key_hash = ?`,
		keyHash,
	).Scan(&key.ID, &key.UserID, &key.KeyHash, &key.KeyPrefix, &key.Name, &lastUsedAt, &expiresAt, &key.CreatedAt, &revokedAt, &key.IsRevoked)

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

func (r *Repository) GetUserAPIKeys(ctx context.Context, userID int64) ([]models.APIKey, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, key_hash, key_prefix, name, last_used_at, expires_at, created_at, revoked_at, is_revoked
		FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var key models.APIKey
		var expiresAt, lastUsedAt, revokedAt sql.NullTime

		if err := rows.Scan(&key.ID, &key.UserID, &key.KeyHash, &key.KeyPrefix, &key.Name, &lastUsedAt, &expiresAt, &key.CreatedAt, &revokedAt, &key.IsRevoked); err != nil {
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

func (r *Repository) RevokeAPIKey(ctx context.Context, keyID int64, userID int64) error {
	var err error
	if r.dbType == config.DBTypeSQLite {
		_, err = r.db.ExecContext(ctx,
			`UPDATE api_keys SET is_revoked = 1, revoked_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
			keyID, userID,
		)
	} else {
		_, err = r.db.ExecContext(ctx,
			`UPDATE api_keys SET is_revoked = true, revoked_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
			keyID, userID,
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

func (r *Repository) DeleteAPIKey(ctx context.Context, keyID int64, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM api_keys WHERE id = ? AND user_id = ?`,
		keyID, userID,
	)
	return err
}
