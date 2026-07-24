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

// Wallet record
pub struct Wallet {
pub:
	address string
	status  string
	balance f64
	tokens  string
}

// Demo wallets (demo data)
pub const demo_wallets = [
	Wallet{'0x742d35Cc6634C0532925a3b844Bc9e7595f1B2Eb', 'hacked', 12.4532, '["USDT","ETH","LINK"]'},
	Wallet{'0x1234567890abcdef1234567890abcdef12345678', 'hacked', 5.8921, '["BTC","ETH"]'},
	Wallet{'0xAb5801a7D398351b8bE11C439e05C5B3259aeC9b', 'hacked', 34.1234, '["USDT","USDC","DAI"]'},
	Wallet{'0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045', 'hacked', 8.7654, '["ETH","UNI","AAVE"]'},
	Wallet{'0x5B38Da6a701c568545dCfcB03FcB875f56beddC4', 'vulnerable', 2.3456, '["ETH","SHIB"]'},
	Wallet{'0xCA35b7d915458EF540aDe6068dFe2F44E8fa733c', 'vulnerable', 7.8901, '["MATIC","ETH"]'},
	Wallet{'0x1aE0EA34a72D944a8C7603FfB3eC30a6669E454c', 'vulnerable', 1.2345, '["BNB","CAKE"]'},
	Wallet{'0x00000000219ab540356cBB839Cbe05303d7705Fa', 'safe', 156789.1234, '["ETH"]'},
	Wallet{'0xBE0eB53F46cd790Cd13851d5EFf43D12404d33E8', 'safe', 23456.7890, '["ETH"]'},
	Wallet{'0x0716a17FBAeE714f1E6aB0f9d59edbC5f09815C0', 'safe', 15.6789, '["ETH","WBTC"]'},
]

// Global state for use in handlers
__global g_state AppState

// Create AppState with configuration from environment
pub fn create_app_state() AppState {
	mut state := AppState{
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
	if state.config.rate_limit_ttl == 0 { state.config.rate_limit_ttl = 86400 } // 24 hours
	
	g_state = state
	return state
}
