module main

import net
import os
import time
import crypto.sha1
import sync

// Config holds all configuration
struct Config {
mut:
	http_port          int
	ws_port           int
	max_checks_per_ip int
	rate_limit_ttl    int
}

// AppState holds all application state (thread-safe with mutex)
struct AppState {
mut:
	config      Config
	rate_limits map[string]RateLimitEntry
	lock        sync.Mutex
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

// Global state pointer for handlers
__global g_state &AppState

fn main() {
	println('===========================================')
	println('  Wallet Checker Server')
	println('  V Language + Net HTTP + WebSocket')
	println('===========================================')
	
	mut config := Config{
		http_port: os.getenv('PORT').int()
		ws_port: os.getenv('WS_PORT').int()
		max_checks_per_ip: os.getenv('MAX_CHECKS').int()
		rate_limit_ttl: os.getenv('RATE_LIMIT_TTL').int()
	}
	
	if config.http_port == 0 { config.http_port = 8080 }
	if config.ws_port == 0 { config.ws_port = 8081 }
	if config.max_checks_per_ip == 0 { config.max_checks_per_ip = 3 }
	if config.rate_limit_ttl == 0 { config.rate_limit_ttl = 3600 }
	
	// Initialize state
	mut state := AppState{
		config: config
		rate_limits: map[string]RateLimitEntry{}
		lock: sync.new_mutex()
	}
	g_state = &state
	
	// Start HTTP server
	spawn http_server(config.http_port)
	
	// Start WebSocket server
	spawn websocket_server(config.ws_port)
	
	println('')
	println('HTTP Server:     http://localhost:${config.http_port}')
	println('WebSocket:      ws://localhost:${config.ws_port}')
	println('Rate limit:     ${config.max_checks_per_ip} checks per IP per ${config.rate_limit_ttl}s')
	println('HTML:           ./assets/index.html')
	println('')
	println('Press Ctrl+C to stop')
	
	// Keep main thread alive
	for {}
}

fn http_server(port int) {
	addr := '0.0.0.0:${port}'
	mut listener := net.listen_tcp(.ip, addr) or {
		eprintln('HTTP: Failed to listen on ${addr}: ${err}')
		return
	}
	
	for {
		mut conn := listener.accept() or { continue }
		spawn handle_http(mut conn)
	}
}

fn handle_http(mut conn net.TcpConn) {
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
		'/' { serve_html(mut conn) }
		'/api/health' { send_json(mut conn, '{"status":"ok","version":"1.0.0"}') }
		'/api/stats' { send_json(mut conn, get_stats_json()) }
		'/api/wallets' { send_json(mut conn, get_wallets_json()) }
		'/api/rate-limit' {
			remaining := get_remaining(client_ip)
			send_json(mut conn, '{"remaining":${remaining},"max":${g_state.config.max_checks_per_ip},"ttl":${g_state.config.rate_limit_ttl}}')
		}
		else {
			if path.starts_with('/api/wallet/') {
				addr := path.substr(12, path.len)
				
				if !check_rate_limit(client_ip) {
					send_429(mut conn)
					return
				}
				
				result := check_wallet_json(addr)
				remaining := get_remaining(client_ip)
				send_json(mut conn, '{"wallet":${result},"remaining":${remaining}}')
			} else {
				send_404(mut conn)
			}
		}
	}
}

fn serve_html(mut conn net.TcpConn) {
	html := os.read_file('assets/index.html') or {
		send_404(mut conn)
		return
	}
	header := 'HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: ${html.len}\r\n\r\n'
	conn.write_string(header) or {}
	conn.write_string(html) or {}
}

fn send_json(mut conn net.TcpConn, body string) {
	header := 'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nAccess-Control-Allow-Origin: *\r\nContent-Length: ${body.len}\r\n\r\n'
	conn.write_string(header) or {}
	conn.write_string(body) or {}
}

fn send_429(mut conn net.TcpConn) {
	body := '{"error":"Rate limit exceeded","message":"Maximum 3 checks per hour"}'
	header := 'HTTP/1.1 429 Too Many Requests\r\nContent-Type: application/json\r\nAccess-Control-Allow-Origin: *\r\nContent-Length: ${body.len}\r\nRetry-After: 3600\r\n\r\n'
	conn.write_string(header) or {}
	conn.write_string(body) or {}
}

fn send_404(mut conn net.TcpConn) {
	body := '{"error":"Not found"}'
	header := 'HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\nContent-Length: ${body.len}\r\n\r\n'
	conn.write_string(header) or {}
	conn.write_string(body) or {}
}

fn websocket_server(port int) {
	addr := '0.0.0.0:${port}'
	mut listener := net.listen_tcp(.ip, addr) or {
		eprintln('WS: Failed to listen on ${addr}: ${err}')
		return
	}
	
	for {
		mut conn := listener.accept() or { continue }
		spawn handle_ws(mut conn)
	}
}

fn handle_ws(mut conn net.TcpConn) {
	defer { conn.close() or {} }
	
	client_ip := conn.peer_ip() or { '127.0.0.1' }
	
	// Read WebSocket handshake
	mut buf := []u8{len: 8192}
	n := conn.read(mut buf) or { return }
	if n == 0 { return }
	
	request := buf[..n].bytestr()
	
	// Simple WebSocket handshake
	if !request.contains('Upgrade: websocket') {
		return
	}
	
	// Get key for response
	key_line := request.split('\n').filter(it.starts_with('Sec-WebSocket-Key:'))
	if key_line.len == 0 { return }
	
	key := key_line[0].substr(19, key_line[0].len).trim(' ')
	
	// Send handshake response
	accept_key := gen_ws_accept(key)
	hs_response := 'HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept_key}\r\n\r\n'
	conn.write_string(hs_response) or { return }
	
	// WebSocket message loop
	for {
		mut msg_buf := []u8{len: 4096}
		msg_len := conn.read(mut msg_buf) or { break }
		if msg_len < 2 { break }
		
		// Parse WebSocket frame
		if msg_buf[0] != 0x81 { continue } // Only text frames
		
		payload_len := int(msg_buf[1] & 0x7F)
		if payload_len > 125 { continue }
		
		// Extract message
		mut message := msg_buf[2..2 + payload_len].bytestr()
		
		// Handle message
		ws_response := handle_ws_message(message, client_ip)
		
		if ws_response.len > 0 {
			// Send WebSocket frame
			mut frame := []u8{len: 2 + ws_response.len}
			frame[0] = 0x81
			frame[1] = u8(ws_response.len)
			for i, b in ws_response.bytes() {
				frame[2 + i] = b
			}
			conn.write(frame) or { break }
		}
	}
}

fn handle_ws_message(msg string, client_ip string) string {
	if msg == '{"type":"ping"}' || msg == 'ping' {
		return '{"type":"pong"}'
	}
	
	if msg.contains('"type":"check_wallet"') {
		// Check rate limit
		if !check_rate_limit(client_ip) {
			return '{"type":"error","message":"Rate limit exceeded"}'
		}
		
		// Extract address
		idx1 := msg.index('"address":"') or { return '' }
		start := idx1 + 10
		// Find closing quote after start position
		rest := msg[start..msg.len]
		idx2 := rest.index('"') or { return '' }
		addr := msg[start..start + idx2]
		
		result := check_wallet_json(addr)
		remaining := get_remaining(client_ip)
		return '{"type":"wallet_result","wallet":${result},"remaining":${remaining}}'
	}
	
	if msg == '{"type":"get_stats"}' || msg == 'get_stats' {
		return '{"type":"stats",${get_stats_json().substr(1, get_stats_json().len - 1)}}'
	}
	
	if msg == '{"type":"get_wallets"}' || msg == 'get_wallets' {
		return '{"type":"wallets","wallets":${get_wallets_json()}}'
	}
	
	if msg == '{"type":"get_rate_limit"}' || msg == 'get_rate_limit' {
		remaining := get_remaining(client_ip)
		return '{"type":"rate_limit","remaining":${remaining},"max":${g_state.config.max_checks_per_ip}}'
	}
	
	return ''
}

fn gen_ws_accept(key string) string {
	s := key + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11'
	h := sha1.sum(s.bytes())
	return h.hex()
}

fn get_stats_json() string {
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

fn get_wallets_json() string {
	mut items := '['
	for i, w in demo_wallets {
		if i > 0 { items += ',' }
		items += '{"address":"${w.address}","status":"${w.status}","balance":${w.balance},"tokens":"${w.tokens}"}'
	}
	items += ']'
	return items
}

fn check_wallet_json(address string) string {
	for w in demo_wallets {
		if w.address == address {
			return '{"address":"${w.address}","status":"${w.status}","balance":${w.balance},"tokens":"${w.tokens}","source":"database"}'
		}
	}
	idx := address.len % demo_wallets.len
	w := demo_wallets[idx]
	return '{"address":"${address}","status":"${w.status}","balance":${w.balance},"tokens":"${w.tokens}","source":"generated"}'
}

fn check_rate_limit(ip string) bool {
	now := time.now().unix()
	
	g_state.lock.lock()
	defer { g_state.lock.unlock() }
	
	entry := g_state.rate_limits[ip]
	if entry.reset_at == 0 {
		g_state.rate_limits[ip] = RateLimitEntry{
			count: 1
			reset_at: now + i64(g_state.config.rate_limit_ttl)
		}
		return true
	}
	
	if now > entry.reset_at {
		g_state.rate_limits[ip] = RateLimitEntry{
			count: 1
			reset_at: now + i64(g_state.config.rate_limit_ttl)
		}
		return true
	}
	
	if entry.count >= g_state.config.max_checks_per_ip {
		return false
	}
	
	g_state.rate_limits[ip] = RateLimitEntry{
		count: entry.count + 1
		reset_at: entry.reset_at
	}
	return true
}

fn get_remaining(ip string) int {
	now := time.now().unix()
	
	g_state.lock.lock()
	defer { g_state.lock.unlock() }
	
	entry := g_state.rate_limits[ip]
	if entry.reset_at == 0 {
		return g_state.config.max_checks_per_ip
	}
	
	if now > entry.reset_at {
		return g_state.config.max_checks_per_ip
	}
	
	return g_state.config.max_checks_per_ip - entry.count
}
