module app

import os
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

// Wallet record from database
pub struct Wallet {
pub:
	id        int
	address   string
	status    string
	balance   f64
	tokens    string
	created_at string
	updated_at string
}

// Create AppState with configuration from environment
pub fn create_app_state() &AppState {
	mut state := &AppState{
		config: Config{
			http_port: os.getenv('PORT').int()
			ws_port: os.getenv('WS_PORT').int()
			max_checks_per_ip: os.getenv('MAX_CHECKS').int()
			rate_limit_ttl: os.getenv('RATE_LIMIT_TTL').int()
		}
		rate_limits: map[string]RateLimitEntry{}
		lock: sync.new_mutex()
	}
	
	if state.config.http_port == 0 { state.config.http_port = 8080 }
	if state.config.ws_port == 0 { state.config.ws_port = 8081 }
	if state.config.max_checks_per_ip == 0 { state.config.max_checks_per_ip = 3 }
	if state.config.rate_limit_ttl == 0 { state.config.rate_limit_ttl = 86400 }
	
	return state
}
