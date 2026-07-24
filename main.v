module main

import net
import os
import time

// Config holds all configuration (no globals)
struct Config {
mut:
	port               int
	max_checks_per_ip int
	rate_limit_ttl     int
}

// AppState holds all application state
struct AppState {
mut:
	config       Config
	rate_limits  map[string]RateLimitEntry
}

// RateLimitEntry for IP rate limiting
struct RateLimitEntry {
mut:
	count    int
	reset_at i64
}

// Wallet record
struct Wallet {
	address string
	status  string
	balance f64
	tokens  string
}

// Demo wallets
const demo_wallets = [
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

fn main() {
	println('===========================================')
	println('  Wallet Checker Server')
	println('  V Language + Net HTTP')
	println('===========================================')
	
	mut config := Config{
		port: os.getenv('PORT').int()
		max_checks_per_ip: os.getenv('MAX_CHECKS').int()
		rate_limit_ttl: os.getenv('RATE_LIMIT_TTL').int()
	}
	
	if config.port == 0 { config.port = 8080 }
	if config.max_checks_per_ip == 0 { config.max_checks_per_ip = 3 }
	if config.rate_limit_ttl == 0 { config.rate_limit_ttl = 3600 }
	
	// Initialize state
	mut state := AppState{
		config: config
		rate_limits: map[string]RateLimitEntry{}
	}
	
	// Create TCP listener
	addr := '0.0.0.0:${config.port}'
	mut listener := net.listen_tcp(.ip, addr) or {
		eprintln('Failed to listen on ${addr}: ${err}')
		return
	}
	
	println('')
	println('Server starting on port ${config.port}')
	println('Rate limit: ${config.max_checks_per_ip} checks per IP per ${config.rate_limit_ttl}s')
	println('MySQL: Not configured (demo mode)')
	println('')
	
	for {
		mut conn := listener.accept() or { continue }
		handle_connection(mut conn, mut state)
	}
}

fn handle_connection(mut conn net.TcpConn, mut state AppState) {
	defer { conn.close() or {} }
	
	mut buf := []u8{len: 8192}
	n := conn.read(mut buf) or { return }
	if n == 0 { return }
	
	data := buf[..n].clone()
	str_data := data.bytestr()
	
	lines := str_data.split('\n')
	if lines.len == 0 { return }
	
	request_line := lines[0]
	parts := request_line.split(' ')
	if parts.len < 2 { return }
	
	path := parts[1]
	client_ip := conn.peer_ip() or { '127.0.0.1' }
	
	match path {
		'/' { send_html(mut conn) }
		'/api/health' { send_json(mut conn, '{"status":"ok","version":"1.0.0"}') }
		'/api/stats' { send_json(mut conn, get_stats()) }
		'/api/wallets' { send_json(mut conn, get_wallets()) }
		'/api/recent' { send_json(mut conn, get_recent()) }
		'/api/rate-limit' {
			remaining := get_remaining(mut state, client_ip)
			send_json(mut conn, '{"remaining":${remaining},"max":${state.config.max_checks_per_ip},"ttl":${state.config.rate_limit_ttl}}')
		}
		else {
			if path.starts_with('/api/wallet/') {
				addr := path.substr(12, path.len)
				
				if !check_rate_limit(mut state, client_ip) {
					send_429(mut conn)
					return
				}
				
				remaining := get_remaining(mut state, client_ip)
				send_json(mut conn, '{"wallet":${check_wallet(addr)},"remaining":${remaining}}')
			} else {
				send_404(mut conn)
			}
		}
	}
}

fn send_html(mut conn net.TcpConn) {
	html := os.read_file('index.html') or { 
		send_404(mut conn)
		return 
	}
	header := 'HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: ${html.len}\r\n\r\n'
	conn.write(header.bytes()) or {}
	conn.write(html.bytes()) or {}
}

fn send_json(mut conn net.TcpConn, body string) {
	header := 'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nAccess-Control-Allow-Origin: *\r\nContent-Length: ${body.len}\r\n\r\n'
	conn.write(header.bytes()) or {}
	conn.write(body.bytes()) or {}
}

fn send_429(mut conn net.TcpConn) {
	body := '{"error":"Rate limit exceeded","message":"Maximum 3 checks per hour"}'
	header := 'HTTP/1.1 429 Too Many Requests\r\nContent-Type: application/json\r\nAccess-Control-Allow-Origin: *\r\nContent-Length: ${body.len}\r\nRetry-After: 3600\r\n\r\n'
	conn.write(header.bytes()) or {}
	conn.write(body.bytes()) or {}
}

fn send_404(mut conn net.TcpConn) {
	body := '{"error":"Not found"}'
	header := 'HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\nContent-Length: ${body.len}\r\n\r\n'
	conn.write(header.bytes()) or {}
	conn.write(body.bytes()) or {}
}

fn get_stats() string {
	mut hacked := 0
	mut vulnerable := 0
	mut safe := 0
	for w in demo_wallets {
		match w.status {
			'hacked' { hacked++ }
			'vulnerable' { vulnerable++ }
			'safe' { safe++ }
			else {}
		}
	}
	total := hacked + vulnerable + safe
	return '{"hacked":${hacked},"vulnerable":${vulnerable},"safe":${safe},"total":${total},"total_checks":${total * 10 + 50}}'
}

fn get_wallets() string {
	mut items := '['
	for i, w in demo_wallets {
		if i > 0 { items += ',' }
		items += '{"address":"${w.address}","status":"${w.status}","balance":${w.balance},"tokens":${w.tokens}}'
	}
	items += ']'
	return items
}

fn get_recent() string {
	return get_wallets()
}

fn check_wallet(address string) string {
	for w in demo_wallets {
		if w.address == address {
			return '{"address":"${w.address}","status":"${w.status}","balance":${w.balance},"tokens":${w.tokens},"source":"database"}'
		}
	}
	idx := address.len % demo_wallets.len
	w := demo_wallets[idx]
	return '{"address":"${address}","status":"${w.status}","balance":${w.balance},"tokens":${w.tokens},"source":"generated"}'
}

fn check_rate_limit(mut state AppState, ip string) bool {
	now := time.now().unix()
	
	entry := state.rate_limits[ip]
	if entry.reset_at == 0 {
		state.rate_limits[ip] = RateLimitEntry{
			count: 1
			reset_at: now + i64(state.config.rate_limit_ttl)
		}
		return true
	}
	
	if now > entry.reset_at {
		state.rate_limits[ip] = RateLimitEntry{
			count: 1
			reset_at: now + i64(state.config.rate_limit_ttl)
		}
		return true
	}
	
	if entry.count >= state.config.max_checks_per_ip {
		return false
	}
	
	state.rate_limits[ip] = RateLimitEntry{
		count: entry.count + 1
		reset_at: entry.reset_at
	}
	return true
}

fn get_remaining(mut state AppState, ip string) int {
	now := time.now().unix()
	
	entry := state.rate_limits[ip]
	if entry.reset_at == 0 {
		return state.config.max_checks_per_ip
	}
	
	if now > entry.reset_at {
		return state.config.max_checks_per_ip
	}
	
	return state.config.max_checks_per_ip - entry.count
}
