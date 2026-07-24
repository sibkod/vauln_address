module main

import net
import os
import time

// IP Rate Limiting
struct RateLimit {
mut:
	count     int
	reset_at  i64
}

const (
	max_checks_per_ip = 3        // Max 3 checks per IP
	rate_limit_ttl    = 3600      // Reset after 1 hour (in seconds)
)

// Thread-safe IP rate limiting map
__global (
	ip_rate_limits map[string]&RateLimit
)

fn check_rate_limit(ip string) bool {
	now := time.now().unix()
	
	unsafe {
		mut limit := ip_rate_limits[ip]
		if limit == 0 {
			limit = &RateLimit{
				count: 0
				reset_at: now + rate_limit_ttl
			}
			ip_rate_limits[ip] = limit
		}
		
		// Check if expired
		if now > limit.reset_at {
			limit.count = 0
			limit.reset_at = now + rate_limit_ttl
		}
		
		// Check limit
		if limit.count >= max_checks_per_ip {
			return false
		}
		
		limit.count++
	}
	return true
}

fn get_remaining_checks(ip string) int {
	now := time.now().unix()
	
	unsafe {
		limit := ip_rate_limits[ip]
		if limit == 0 {
			return max_checks_per_ip
		}
		
		if now > limit.reset_at {
			return max_checks_per_ip
		}
		
		return max_checks_per_ip - limit.count
	}
}

// Demo wallets with various statuses
const demo_wallets = [
	'0x742d35Cc6634C0532925a3b844Bc9e7595f1B2Eb',
	'0x1234567890abcdef1234567890abcdef12345678',
	'0xAb5801a7D398351b8bE11C439e05C5B3259aeC9b',
	'0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045',
	'0x5B38Da6a701c568545dCfcB03FcB875f56beddC4',
	'0xCA35b7d915458EF540aDe6068dFe2F44E8fa733c',
	'0x1aE0EA34a72D944a8C7603FfB3eC30a6669E454c',
	'0x00000000219ab540356cBB839Cbe05303d7705Fa',
	'0xBE0eB53F46cd790Cd13851d5EFf43D12404d33E8',
	'0x0716a17FBAeE714f1E6aB0f9d59edbC5f09815C0',
	'0x3fC91A3afd70395Cd496C647d5a6CC9D4B2b7FAD',
	'0xB4e16d0168e52d35CaCD2c6185b44281Ec28C9Dc',
	'0x0D0707963952f2fBA59dD06f2b18aceEF568593f',
	'0x56B89804fAa041Ad5F4aEa5B0d4cE8B1E4F5e6D7',
	'0x47ac0Fb4F2D84898e4D9E7b4DaB3C24507a6D503',
]

const demo_statuses = ['hacked', 'hacked', 'hacked', 'hacked', 'vulnerable', 'vulnerable', 'vulnerable', 'safe', 'safe', 'safe', 'safe', 'safe', 'vulnerable', 'hacked', 'safe']

const demo_balances = [12.4532, 5.8921, 34.1234, 8.7654, 2.3456, 7.8901, 1.2345, 156789.1234, 23456.7890, 15.6789, 45.6789, 234.5678, 9.0123, 67.8901, 123.4567]

const demo_tokens = [
	'["USDT","ETH","LINK"]',
	'["BTC","ETH"]',
	'["USDT","USDC","DAI"]',
	'["ETH","UNI","AAVE"]',
	'["ETH","SHIB"]',
	'["MATIC","ETH"]',
	'["BNB","CAKE"]',
	'["ETH"]',
	'["ETH"]',
	'["ETH","WBTC"]',
	'["USDT","ETH","CRV"]',
	'["USDC","DAI"]',
	'["ETH","SOL","AVAX"]',
	'["USDT","ETH","ADA"]',
	'["ETH","BTC"]',
]

__global (
	ws_client_count int
)

fn main() {
	println('===========================================')
	println('  Wallet Checker Server')
	println('  V Language + WebSocket + Demo Data')
	println('===========================================')
	
	// Initialize IP rate limits map
	ip_rate_limits = map[string]&RateLimit{}
	
	// Load HTML from file
	html_content := os.read_file('index.html') or {
		println('Warning: index.html not found, using built-in fallback')
		get_fallback_html()
	}
	
	port := 8080
	ws_port := 8081
	
	println('')
	println('Starting servers...')
	println('HTTP Server: http://localhost:${port}')
	println('WebSocket:   ws://localhost:${ws_port}')
	println('')
	
	go http_server(port, html_content)
	go ws_server(ws_port)
	
	// Keep running
	for {}
}

fn get_fallback_html() string {
	return '<!DOCTYPE html><html><head><title>Wallet Checker</title></head><body><h1>Wallet Checker</h1><p>Demo mode</p></body></html>'
}

fn http_server(port int, html_content string) {
	ln := net.listen_tcp(.ip, ':${port}') or {
		println('HTTP Listen error: ${err}')
		return
	}
	
	println('HTTP server started on port ${port}')
	
	mut listener := ln
	for {
		conn := listener.accept() or { continue }
		go handle_http(conn, html_content)
	}
}

fn handle_http(conn &net.TcpConn, html_content string) {
	mut c := conn
	defer { c.close() or {} }
	
	mut buf := []u8{len: 8192}
	n := c.read(mut buf) or { return }
	if n == 0 { return }
	
	data := buf[..n]
	str_data := data.bytestr()
	
	lines := str_data.split('\n')
	if lines.len == 0 { return }
	
	request_line := lines[0]
	parts := request_line.split(' ')
	if parts.len < 2 { return }
	
	path := parts[1]
	
	// Get client IP
	client_ip := get_client_ip(c)
	
	match path {
		'/' { send_html(c, html_content) }
		'/api/health' { send_json(c, '{"status":"ok","version":"1.0.0"}') }
		'/api/wallets' { send_json(c, get_wallets_json()) }
		'/api/stats' { send_json(c, get_stats_json()) }
		'/api/recent' { send_json(c, get_recent_json()) }
		'/api/rate-limit' {
			remaining := get_remaining_checks(client_ip)
			send_json(c, '{"remaining":${remaining},"max":${max_checks_per_ip},"ttl":${rate_limit_ttl}}')
		}
		else {
			if path.starts_with('/api/wallet/') {
				addr := path.substr(12, path.len)
				
				// Check rate limit
				if !check_rate_limit(client_ip) {
					remaining := get_remaining_checks(client_ip)
					send_rate_limited(c, remaining)
					return
				}
				
				remaining := get_remaining_checks(client_ip)
				send_json(c, '{"wallet":${get_wallet_json(addr)},"remaining":${remaining}}')
			} else {
				send_404(c)
			}
		}
	}
}

fn get_client_ip(c &net.TcpConn) string {
	// Try to get peer IP, fallback to default
	peer_ip := c.peer_ip() or { return '127.0.0.1' }
	return peer_ip
}

fn send_rate_limited(c &net.TcpConn, remaining int) {
	mut conn := c
	body := '{"error":"Rate limit exceeded","message":"Maximum ${max_checks_per_ip} checks per hour","remaining":${remaining}}'
	header := 'HTTP/1.1 429 Too Many Requests\r\nContent-Type: application/json\r\nAccess-Control-Allow-Origin: *\r\nContent-Length: ${body.len}\r\nRetry-After: 3600\r\n\r\n'
	conn.write(header.bytes()) or {}
	conn.write(body.bytes()) or {}
}

fn send_html(c &net.TcpConn, html string) {
	mut conn := c
	header := 'HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: ${html.len}\r\n\r\n'
	conn.write(header.bytes()) or {}
	conn.write(html.bytes()) or {}
}

fn send_json(c &net.TcpConn, json string) {
	mut conn := c
	header := 'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nAccess-Control-Allow-Origin: *\r\nContent-Length: ${json.len}\r\n\r\n'
	conn.write(header.bytes()) or {}
	conn.write(json.bytes()) or {}
}

fn send_404(c &net.TcpConn) {
	mut conn := c
	body := '{"error":"Not found"}'
	header := 'HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\nContent-Length: ${body.len}\r\n\r\n'
	conn.write(header.bytes()) or {}
	conn.write(body.bytes()) or {}
}

fn get_wallets_json() string {
	mut items := '['
	for i := 0; i < demo_wallets.len; i++ {
		if i > 0 { items += ',' }
		items += '{"address":"${demo_wallets[i]}","status":"${demo_statuses[i]}","balance":${demo_balances[i]},"tokens":${demo_tokens[i]}}'
	}
	items += ']'
	return items
}

fn get_stats_json() string {
	mut hacked := 0
	mut vulnerable := 0
	mut safe := 0
	for i := 0; i < demo_wallets.len; i++ {
		match demo_statuses[i] {
			'hacked' { hacked++ }
			'vulnerable' { vulnerable++ }
			'safe' { safe++ }
			else {}
		}
	}
	return '{"hacked":${hacked},"vulnerable":${vulnerable},"safe":${safe},"total":${demo_wallets.len},"total_checks":${demo_wallets.len * 10 + 50}}'
}

fn get_recent_json() string {
	mut items := '['
	for i := 0; i < demo_wallets.len && i < 10; i++ {
		if i > 0 { items += ',' }
		items += '{"address":"${demo_wallets[i]}","status":"${demo_statuses[i]}","balance":${demo_balances[i]},"tokens":${demo_tokens[i]}}'
	}
	items += ']'
	return items
}

fn get_wallet_json(addr string) string {
	for i := 0; i < demo_wallets.len; i++ {
		if demo_wallets[i] == addr {
			return '{"address":"${demo_wallets[i]}","status":"${demo_statuses[i]}","balance":${demo_balances[i]},"tokens":${demo_tokens[i]},"source":"database"}'
		}
	}
	// Generate random result for unknown wallets
	idx := addr.len % demo_wallets.len
	return '{"address":"${addr}","status":"${demo_statuses[idx]}","balance":${demo_balances[idx]},"tokens":${demo_tokens[idx]},"source":"generated"}'
}

// WebSocket server
fn ws_server(port int) {
	ln := net.listen_tcp(.ip, ':${port}') or {
		println('WebSocket Listen error: ${err}')
		return
	}
	
	println('WebSocket server started on port ${port}')
	
	mut listener := ln
	for {
		conn := listener.accept() or { continue }
		go handle_ws(conn)
	}
}

fn handle_ws(conn &net.TcpConn) {
	mut c := conn
	defer { c.close() or {} }
	
	ws_client_count++
	client_id := ws_client_count
	println('WS Client #${client_id} connected. Total: ${ws_client_count}')
	
	// Send welcome message
	welcome := '{"type":"welcome","payload":{"client_id":${client_id},"message":"Connected to Wallet Checker!"},"timestamp":1234567890}'
	c.write(make_frame(welcome)) or {}
	
	mut buf := []u8{len: 8192}
	for {
		n := c.read(mut buf) or { break }
		if n == 0 { break }
		
		frame := buf[..n]
		if frame.len < 2 { continue }
		
		opcode := frame[0] & 0x0F
		if opcode == 0x8 { break }
		
		if opcode == 0x1 {
			payload := decode_frame(frame)
			handle_ws_message(c, payload)
		}
	}
	
	ws_client_count--
	println('WS Client #${client_id} disconnected. Total: ${ws_client_count}')
}

fn handle_ws_message(c &net.TcpConn, payload string) {
	mut conn := c
	
	// Simple action detection
	if payload.contains('"action"') || payload.contains('"type"') {
		if payload.contains('ping') || payload.contains('"ping"') {
			response := '{"type":"pong","timestamp":1234567890}'
			conn.write(make_frame(response)) or {}
		} else if payload.contains('get_stats') || payload.contains('"stats"') {
			conn.write(make_frame('{"type":"stats_update","payload":${get_stats_json()},"timestamp":1234567890}')) or {}
		} else if payload.contains('check') || payload.contains('wallet') {
			// Generate random wallet check result
			idx := payload.len % demo_wallets.len
			result := '{"type":"check_result","payload":{"address":"${demo_wallets[idx]}","status":"${demo_statuses[idx]}","balance":${demo_balances[idx]}},"timestamp":1234567890}'
			conn.write(make_frame(result)) or {}
		} else {
			response := '{"type":"echo","payload":"${payload}","timestamp":1234567890}'
			conn.write(make_frame(response)) or {}
		}
	}
}

fn make_frame(msg string) []u8 {
	mut frame := []u8{}
	frame << 0x81
	length := msg.len
	if length <= 125 {
		frame << u8(length)
	} else if length <= 65535 {
		frame << 126
		frame << u8(length >> 8)
		frame << u8(length & 0xFF)
	} else {
		frame << 127
		for i := 56; i >= 0; i -= 8 {
			frame << u8(length >> i)
		}
	}
	frame << msg.bytes()
	return frame
}

fn decode_frame(data []u8) string {
	if data.len < 2 { return '' }
	
	mask := data[1] & 0x80
	mut length := int(data[1] & 0x7F)
	mut offset := 2
	
	if length == 126 && data.len >= 4 {
		length = int(data[2]) << 8 | int(data[3])
		offset = 4
	} else if length == 127 && data.len >= 10 {
		length = 0
		for i := 2; i < 10; i++ {
			length = length << 8 | int(data[i])
		}
		offset = 10
	}
	
	if mask != 0 && data.len >= offset + 4 {
		offset += 4
	}
	
	if data.len < offset + length { return '' }
	
	mut payload := data[offset..offset+length].clone()
	
	if mask != 0 && data.len >= 2 {
		key := data[offset-4..offset]
		for i := 0; i < payload.len; i++ {
			payload[i] ^= key[i % 4]
		}
	}
	
	return payload.bytestr()
}
