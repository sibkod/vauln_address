module app

import os
import db.mysql

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

// Connect to MySQL using db.mysql
pub fn db_connect(cfg DBConfig) !mysql.DB {
	return mysql.connect(mysql.Config{
		host: cfg.host
		port: u32(cfg.port)
		user: cfg.user
		password: cfg.password
		dbname: cfg.database
	})!
}

// Init database and run migrations
pub fn init_database(cfg DBConfig) ! {
	// Connect without database first
	mut conn := mysql.connect(mysql.Config{
		host: cfg.host
		port: u32(cfg.port)
		user: cfg.user
		password: cfg.password
	})!
	
	// Create database
	conn.execute('CREATE DATABASE IF NOT EXISTS ${cfg.database}') or {}
	conn.close()!
	
	// Connect to new database
	db := db_connect(cfg)!
	
	// Create tables
	db.execute('CREATE TABLE IF NOT EXISTS wallets (
		id INT AUTO_INCREMENT PRIMARY KEY,
		address VARCHAR(255) NOT NULL UNIQUE,
		status ENUM("hacked", "vulnerable", "safe") NOT NULL DEFAULT "safe",
		balance DECIMAL(18, 8) DEFAULT 0,
		tokens JSON,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4') or {}
	
	db.execute('CREATE TABLE IF NOT EXISTS check_logs (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		wallet_id INT,
		ip_address VARCHAR(45) NOT NULL,
		check_result ENUM("hacked", "vulnerable", "safe", "not_found") NOT NULL,
		checked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE SET NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4') or {}
	
	println('Database tables created!')
	
	// Seed demo data
	seed_demo_data(db)!
	
	db.close()!
}

// Demo wallets
const demo_wallets = [
	Wallet{address: '0x742d35Cc6634C0532925a3b844Bc9e7595f1B2Eb', status: 'hacked', balance: 12.4532, tokens: '["USDT","ETH","LINK"]'},
	Wallet{address: '0x1234567890abcdef1234567890abcdef12345678', status: 'hacked', balance: 5.8921, tokens: '["BTC","ETH"]'},
	Wallet{address: '0xAb5801a7D398351b8bE11C439e05C5B3259aeC9b', status: 'hacked', balance: 34.1234, tokens: '["USDT","USDC","DAI"]'},
	Wallet{address: '0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045', status: 'hacked', balance: 8.7654, tokens: '["ETH","UNI","AAVE"]'},
	Wallet{address: '0x5B38Da6a701c568545dCfcB03FcB875f56beddC4', status: 'vulnerable', balance: 2.3456, tokens: '["ETH","SHIB"]'},
	Wallet{address: '0xCA35b7d915458EF540aDe6068dFe2F44E8fa733c', status: 'vulnerable', balance: 7.8901, tokens: '["MATIC","ETH"]'},
	Wallet{address: '0x1aE0EA34a72D944a8C7603FfB3eC30a6669E454c', status: 'vulnerable', balance: 1.2345, tokens: '["BNB","CAKE"]'},
	Wallet{address: '0x00000000219ab540356cBB839Cbe05303d7705Fa', status: 'safe', balance: 156789.1234, tokens: '["ETH"]'},
	Wallet{address: '0xBE0eB53F46cd790Cd13851d5EFf43D12404d33E8', status: 'safe', balance: 23456.7890, tokens: '["ETH"]'},
	Wallet{address: '0x0716a17FBAeE714f1E6aB0f9d59edbC5f09815C0', status: 'safe', balance: 15.6789, tokens: '["ETH","WBTC"]'},
]

// Seed demo data
pub fn seed_demo_data(db mysql.DB) ! {
	for w in demo_wallets {
		// Escape quotes in tokens JSON
		safe_tokens := w.tokens.replace('"', '\\"')
		query := "INSERT INTO wallets (address, status, balance, tokens) VALUES ('${w.address}', '${w.status}', ${w.balance}, '${safe_tokens}') ON DUPLICATE KEY UPDATE status='${w.status}', balance=${w.balance}, tokens='${safe_tokens}'"
		db.execute(query) or {
			if !err.msg().contains('Duplicate') {
				println('Seed error: ${err}')
			}
		}
	}
	println('Demo data seeded!')
}
