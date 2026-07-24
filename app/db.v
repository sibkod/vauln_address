module app

import os

// DBConfig holds MySQL connection settings
pub struct DBConfig {
pub mut:
	host     string
	port     int
	user     string
	password string
	database string
}

// Get DB config from environment
pub fn get_db_config() DBConfig {
	return DBConfig{
		host: os.getenv('DB_HOST')
		port: os.getenv('DB_PORT').int()
		user: os.getenv('DB_USER')
		password: os.getenv('DB_PASSWORD')
		database: os.getenv('DB_NAME')
	}
}

// Migration 1: Create wallets table
pub fn migration_create_wallets() string {
	return "CREATE TABLE IF NOT EXISTS wallets (
		id INT AUTO_INCREMENT PRIMARY KEY,
		address VARCHAR(255) NOT NULL UNIQUE,
		status ENUM('hacked', 'vulnerable', 'safe') NOT NULL DEFAULT 'safe',
		balance DECIMAL(18, 8) DEFAULT 0,
		tokens JSON,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_address (address),
		INDEX idx_status (status)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci"
}

// Migration 2: Create check_logs table
pub fn migration_create_check_logs() string {
	return "CREATE TABLE IF NOT EXISTS check_logs (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		wallet_id INT,
		ip_address VARCHAR(45) NOT NULL,
		check_result ENUM('hacked', 'vulnerable', 'safe', 'not_found') NOT NULL,
		checked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE SET NULL,
		INDEX idx_ip (ip_address),
		INDEX idx_checked_at (checked_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci"
}

// Get all migrations
pub fn get_migrations() []string {
	return [
		migration_create_wallets(),
		migration_create_check_logs(),
	]
}

// Demo data for wallets
pub const demo_data = [
	WalletData{
		address: '0x742d35Cc6634C0532925a3b844Bc9e7595f1B2Eb'
		status: 'hacked'
		balance: 12.4532
		tokens: '["USDT","ETH","LINK"]'
	},
	WalletData{
		address: '0x1234567890abcdef1234567890abcdef12345678'
		status: 'hacked'
		balance: 5.8921
		tokens: '["BTC","ETH"]'
	},
	WalletData{
		address: '0xAb5801a7D398351b8bE11C439e05C5B3259aeC9b'
		status: 'hacked'
		balance: 34.1234
		tokens: '["USDT","USDC","DAI"]'
	},
	WalletData{
		address: '0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045'
		status: 'hacked'
		balance: 8.7654
		tokens: '["ETH","UNI","AAVE"]'
	},
	WalletData{
		address: '0x5B38Da6a701c568545dCfcB03FcB875f56beddC4'
		status: 'vulnerable'
		balance: 2.3456
		tokens: '["ETH","SHIB"]'
	},
	WalletData{
		address: '0xCA35b7d915458EF540aDe6068dFe2F44E8fa733c'
		status: 'vulnerable'
		balance: 7.8901
		tokens: '["MATIC","ETH"]'
	},
	WalletData{
		address: '0x1aE0EA34a72D944a8C7603FfB3eC30a6669E454c'
		status: 'vulnerable'
		balance: 1.2345
		tokens: '["BNB","CAKE"]'
	},
	WalletData{
		address: '0x00000000219ab540356cBB839Cbe05303d7705Fa'
		status: 'safe'
		balance: 156789.1234
		tokens: '["ETH"]'
	},
	WalletData{
		address: '0xBE0eB53F46cd790Cd13851d5EFf43D12404d33E8'
		status: 'safe'
		balance: 23456.7890
		tokens: '["ETH"]'
	},
	WalletData{
		address: '0x0716a17FBAeE714f1E6aB0f9d59edbC5f09815C0'
		status: 'safe'
		balance: 15.6789
		tokens: '["ETH","WBTC"]'
	},
]

// WalletData for inserting demo data
pub struct WalletData {
	address string
	status  string
	balance f64
	tokens  string
}

// Generate SQL for inserting demo data
pub fn get_demo_insert_sql() []string {
	mut sqls := []string{}
	for w in demo_data {
		query := "INSERT INTO wallets (address, status, balance, tokens) VALUES ('${w.address}', '${w.status}', ${w.balance}, '${w.tokens}') ON DUPLICATE KEY UPDATE status='${w.status}', balance=${w.balance}, tokens='${w.tokens}'"
		sqls << query
	}
	return sqls
}

// SQL to get all wallets
pub const sql_get_all_wallets = 'SELECT id, address, status, balance, tokens, created_at, updated_at FROM wallets ORDER BY status, id'

// SQL to get wallet by address
pub const sql_get_wallet_by_address = 'SELECT id, address, status, balance, tokens, created_at, updated_at FROM wallets WHERE address = ?'

// SQL to log a check
pub const sql_log_check = 'INSERT INTO check_logs (wallet_id, ip_address, check_result) VALUES (?, ?, ?)'
