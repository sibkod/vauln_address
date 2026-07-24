module app

import fasthttp
import net.websocket
import os
import time
import mysql

// HTTP Handler using fasthttp append_handler
pub fn create_http_handler(state &AppState) fasthttp.AppendHandler {
	return fn (req fasthttp.HttpRequest, mut out []u8, ws voidptr, mut ctl fasthttp.ResponseControl) fasthttp.Step {
		// Extract path from buffer using Slice
		path_bytes := req.buffer[req.path.start..req.path.start + req.path.len]
		path := path_bytes.bytestr()
		
		if path == '/' {
			html := os.read_file('templates/index.html') or {
				return build_404(mut out)
			}
			return build_html(mut out, html)
		}
		
		if path == '/api/health' {
			body := '{"status":"ok","version":"1.0.0","engine":"fasthttp"}'
			return build_json(mut out, body)
		}
		
		return .done
	}
}

fn build_html(mut out []u8, html string) fasthttp.Step {
	header := 'HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: ${html.len}\r\n\r\n'
	out << header.bytes()
	out << html.bytes()
	return .done
}

fn build_json(mut out []u8, body string) fasthttp.Step {
	header := 'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nAccess-Control-Allow-Origin: *\r\nContent-Length: ${body.len}\r\n\r\n'
	out << header.bytes()
	out << body.bytes()
	return .done
}

fn build_404(mut out []u8) fasthttp.Step {
	body := '{"error":"Not found"}'
	header := 'HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\nContent-Length: ${body.len}\r\n\r\n'
	out << header.bytes()
	out << body.bytes()
	return .done
}

// WebSocket Handler
pub fn create_websocket_server(mut state AppState, db &mysql.Connection, port int) &websocket.Server {
	mut ws_server := websocket.new_server(.ip, port, '/ws')
	
	ws_server.on_message(fn [mut state, db] (mut ws websocket.Client, msg &websocket.Message) ! {
		data := msg.payload.bytestr()
		if data == '' { return }
		
		// Get client IP from connection
		client_ip := ws.conn.peer_ip() or { '127.0.0.1' }
		
		// Handle message using state and db
		response := handle_ws_message(data, mut state, db, client_ip)
		
		if response.len > 0 {
			ws.write_string(response) or { return }
		}
	})
	
	ws_server.on_close(fn (mut ws websocket.Client, code int, reason string) ! {
		// Client disconnected
	})
	
	return ws_server
}

fn handle_ws_message(msg string, mut state AppState, db &mysql.Connection, client_ip string) string {
	if msg == '{"type":"ping"}' || msg == 'ping' {
		return '{"type":"pong"}'
	}
	
	if msg.contains('"type":"check_wallet"') {
		// Check rate limit
		if !check_rate_limit(mut state, client_ip) {
			return '{"type":"error","message":"Rate limit exceeded"}'
		}
		
		// Extract address
		idx1 := msg.index('"address":"') or { return '' }
		start := idx1 + 10
		rest := msg[start..msg.len]
		idx2 := rest.index('"') or { return '' }
		addr := msg[start..start + idx2]
		
		// Query wallet from database
		result := check_wallet_from_db(db, addr) or { return '{"type":"error","message":"Database error"}' }
		remaining := get_remaining(&state, client_ip)
		return '{"type":"wallet_result","wallet":${result},"remaining":${remaining}}'
	}
	
	if msg == '{"type":"get_stats"}' || msg == 'get_stats' {
		stats := get_stats_from_db(db) or { '{"type":"stats","error":"Failed to get stats"}' }
		return '{"type":"stats",${stats}}'
	}
	
	if msg == '{"type":"get_wallets"}' || msg == 'get_wallets' {
		wallets := get_wallets_from_db(db) or { '[]' }
		return '{"type":"wallets","wallets":${wallets}}'
	}
	
	if msg == '{"type":"get_rate_limit"}' || msg == 'get_rate_limit' {
		remaining := get_remaining(&state, client_ip)
		return '{"type":"rate_limit","remaining":${remaining},"max":${state.config.max_checks_per_ip}}'
	}
	
	return ''
}

// Database functions
fn check_wallet_from_db(db &mysql.Connection, address string) !string {
	// Query wallet by address
	rows := db.query(sql_get_wallet_by_address, [address])!
	defer { rows.free() }
	
	if rows.next()! {
		status := rows.varchar_by_index(2)!
		balance := rows.varchar_by_index(3)!
		tokens := rows.varchar_by_index(4)!
		return '{"address":"${address}","status":"${status}","balance":${balance},"tokens":"${tokens}","source":"database"}'
	}
	
	// Not found - return random status
	idx := address.len % demo_data.len
	w := demo_data[idx]
	return '{"address":"${address}","status":"${w.status}","balance":${w.balance},"tokens":"${w.tokens}","source":"generated"}'
}

fn get_stats_from_db(db &mysql.Connection) !string {
	// Get counts by status
	hacked := db.query('SELECT COUNT(*) FROM wallets WHERE status = "hacked"', [])!.single_string() or { '0' }
	vulnerable := db.query('SELECT COUNT(*) FROM wallets WHERE status = "vulnerable"', [])!.single_string() or { '0' }
	safe := db.query('SELECT COUNT(*) FROM wallets WHERE status = "safe"', [])!.single_string() or { '0' }
	total := db.query('SELECT COUNT(*) FROM wallets', [])!.single_string() or { '0' }
	total_checks := db.query('SELECT COUNT(*) FROM check_logs', [])!.single_string() or { '0' }
	
	return '{"hacked":${hacked},"vulnerable":${vulnerable},"safe":${safe},"total":${total},"total_checks":${total_checks}}'
}

fn get_wallets_from_db(db &mysql.Connection) !string {
	rows := db.query(sql_get_all_wallets, [])!
	defer { rows.free() }
	
	mut items := '['
	mut first := true
	for rows.next()! {
		if !first { items += ',' }
		first = false
		address := rows.varchar_by_index(1)!
		status := rows.varchar_by_index(2)!
		balance := rows.varchar_by_index(3)!
		tokens := rows.varchar_by_index(4)!
		items += '{"address":"${address}","status":"${status}","balance":${balance},"tokens":"${tokens}"}'
	}
	items += ']'
	return items
}

// Rate limiting using mutex lock
fn check_rate_limit(mut state AppState, ip string) bool {
	now := time.now().unix()
	
	state.lock.lock()
	entry := state.rate_limits[ip]
	if entry.reset_at == 0 {
		state.rate_limits[ip] = RateLimitEntry{
			count: 1
			reset_at: now + i64(state.config.rate_limit_ttl)
		}
		state.lock.unlock()
		return true
	}
	
	if now > entry.reset_at {
		state.rate_limits[ip] = RateLimitEntry{
			count: 1
			reset_at: now + i64(state.config.rate_limit_ttl)
		}
		state.lock.unlock()
		return true
	}
	
	if entry.count >= state.config.max_checks_per_ip {
		state.lock.unlock()
		return false
	}
	
	state.rate_limits[ip] = RateLimitEntry{
		count: entry.count + 1
		reset_at: entry.reset_at
	}
	state.lock.unlock()
	return true
}

fn get_remaining(state &AppState, ip string) int {
	now := time.now().unix()
	
	state.lock.lock()
	entry := state.rate_limits[ip]
	remaining := if entry.reset_at == 0 || now > entry.reset_at {
		state.config.max_checks_per_ip
	} else {
		state.config.max_checks_per_ip - entry.count
	}
	state.lock.unlock()
	return remaining
}
