module app

import fasthttp
import net.websocket
import os
import time

// HTTP Handler using fasthttp classic handler
pub fn create_http_handler() fn (fasthttp.HttpRequest) !fasthttp.HttpResponse {
	return fn (req fasthttp.HttpRequest) !fasthttp.HttpResponse {
		// Extract path from buffer using Slice
		path_bytes := req.buffer[req.path.start..req.path.start + req.path.len]
		path := path_bytes.bytestr()
		
		match path {
			'/' {
				html := os.read_file('templates/index.html') or {
					return error('index.html not found')
				}
				return build_response(html, 'text/html')
			}
			'/api/health' {
				body := '{"status":"ok","version":"1.0.0","engine":"fasthttp"}'
				return build_json_response(body)
			}
			'/api/stats' {
				body := get_stats_json()
				return build_json_response(body)
			}
			'/api/wallets' {
				body := get_wallets_json()
				return build_json_response(body)
			}
			'/api/rate-limit' {
				ip := get_client_ip(req)
				remaining := get_remaining(ip)
				body := '{"remaining":${remaining},"max":${g_state.config.max_checks_per_ip},"ttl":${g_state.config.rate_limit_ttl}}'
				return build_json_response(body)
			}
			else {
				if path.starts_with('/api/wallet/') {
					addr := path[12..path.len]
					ip := get_client_ip(req)
					
					if !check_rate_limit(ip) {
						return build_429_response()
					}
					
					result := check_wallet_json(addr)
					remaining := get_remaining(ip)
					body := '{"wallet":${result},"remaining":${remaining}}'
					return build_json_response(body)
				}
				return build_404_response()
			}
		}
	}
}

fn build_response(content string, content_type string) !fasthttp.HttpResponse {
	header := 'HTTP/1.1 200 OK\r\nContent-Type: ${content_type}\r\nContent-Length: ${content.len}\r\n\r\n'
	return fasthttp.HttpResponse{
		content: (header + content).bytes()
	}
}

fn build_json_response(body string) !fasthttp.HttpResponse {
	header := 'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nAccess-Control-Allow-Origin: *\r\nContent-Length: ${body.len}\r\n\r\n'
	return fasthttp.HttpResponse{
		content: (header + body).bytes()
	}
}

fn build_404_response() !fasthttp.HttpResponse {
	body := '{"error":"Not found"}'
	header := 'HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\nContent-Length: ${body.len}\r\n\r\n'
	return fasthttp.HttpResponse{
		content: (header + body).bytes()
	}
}

fn build_429_response() !fasthttp.HttpResponse {
	body := '{"error":"Rate limit exceeded","message":"Maximum 3 checks per day"}'
	header := 'HTTP/1.1 429 Too Many Requests\r\nContent-Type: application/json\r\nAccess-Control-Allow-Origin: *\r\nContent-Length: ${body.len}\r\nRetry-After: 86400\r\n\r\n'
	return fasthttp.HttpResponse{
		content: (header + body).bytes()
	}
}

fn get_client_ip(req fasthttp.HttpRequest) string {
	// Get headers from buffer - they come after the request line
	headers_start := req.header_fields.start
	headers_len := req.header_fields.len
	if headers_len == 0 {
		return '127.0.0.1'
	}
	
	headers_bytes := req.buffer[headers_start..headers_start + headers_len]
	headers := headers_bytes.bytestr()
	
	// Simple parsing for X-Forwarded-For and X-Real-IP
	lines := headers.split('\n')
	for line in lines {
		if line.starts_with('X-Forwarded-For:') {
			ip := line.substr(17, line.len).trim(' ')
			if ip.contains(',') {
				return ip.split(',')[0].trim(' ')
			}
			return ip.trim(' ')
		}
		if line.starts_with('X-Real-IP:') {
			return line.substr(11, line.len).trim(' ')
		}
	}
	return '127.0.0.1'
}

// WebSocket Handler
pub fn create_websocket_server(port int) &websocket.Server {
	mut ws_server := websocket.new_server(.ip, port, '')
	
	ws_server.on_message(fn (mut ws websocket.Client, msg &websocket.Message) ! {
		data := msg.payload.bytestr()
		if data == '' { return }
		
		// Get client IP from connection
		client_ip := ws.conn.peer_ip() or { '127.0.0.1' }
		
		// Handle message
		response := handle_ws_message(data, client_ip)
		
		if response.len > 0 {
			ws.write_string(response) or { return }
		}
	})
	
	ws_server.on_close(fn (mut ws websocket.Client, code int, reason string) ! {
		// Client disconnected
	})
	
	return ws_server
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

// JSON helpers
pub fn get_stats_json() string {
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

pub fn get_wallets_json() string {
	mut items := '['
	for i, w in demo_wallets {
		if i > 0 { items += ',' }
		items += '{"address":"${w.address}","status":"${w.status}","balance":${w.balance},"tokens":"${w.tokens}"}'
	}
	items += ']'
	return items
}

pub fn check_wallet_json(address string) string {
	for w in demo_wallets {
		if w.address == address {
			return '{"address":"${w.address}","status":"${w.status}","balance":${w.balance},"tokens":"${w.tokens}","source":"database"}'
		}
	}
	idx := address.len % demo_wallets.len
	w := demo_wallets[idx]
	return '{"address":"${address}","status":"${w.status}","balance":${w.balance},"tokens":"${w.tokens}","source":"generated"}'
}

// Rate limiting - uses thread-safe access to g_state
fn check_rate_limit(ip string) bool {
	now := time.now().unix()
	
	// Mutex lock/unlock for thread safety
	g_state.lock.lock()
	entry := g_state.rate_limits[ip]
	if entry.reset_at == 0 {
		g_state.rate_limits[ip] = RateLimitEntry{
			count: 1
			reset_at: now + i64(g_state.config.rate_limit_ttl)
		}
		g_state.lock.unlock()
		return true
	}
	
	if now > entry.reset_at {
		g_state.rate_limits[ip] = RateLimitEntry{
			count: 1
			reset_at: now + i64(g_state.config.rate_limit_ttl)
		}
		g_state.lock.unlock()
		return true
	}
	
	if entry.count >= g_state.config.max_checks_per_ip {
		g_state.lock.unlock()
		return false
	}
	
	g_state.rate_limits[ip] = RateLimitEntry{
		count: entry.count + 1
		reset_at: entry.reset_at
	}
	g_state.lock.unlock()
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
