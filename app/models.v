module app

import sync

// Config holds all configuration
pub struct Config {
pub mut:
	http_port          int
	ws_port           int
	max_checks_per_ip int
	rate_limit_ttl    int
}

// RateLimitEntry for IP rate limiting
pub struct RateLimitEntry {
pub mut:
	count    int
	reset_at i64
}

// AppState holds all application state (thread-safe with mutex)
pub struct AppState {
pub mut:
	config      Config
	rate_limits map[string]RateLimitEntry
	lock        sync.Mutex
}

// Wallet ORM model
@[table: 'wallets']
pub struct Wallet {
pub:
	id         int    @[default: 0; primary; sql_type: 'INT']
	address    string @[unique; sql_type: 'VARCHAR(255)']
	status     string @[default: 'safe']
	balance    f64    @[default: 0.0]
	tokens     string @[sql_type: 'JSON']
	created_at string @[default: '']
	updated_at string @[default: '']
}

// Create AppState with configuration from environment
pub fn create_app_state() &AppState {
	return &AppState{
		config: Config{
			http_port: 8080
			ws_port: 8081
			max_checks_per_ip: 3
			rate_limit_ttl: 86400
		}
		rate_limits: map[string]RateLimitEntry{}
		lock: sync.new_mutex()
	}
}
