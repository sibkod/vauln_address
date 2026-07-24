module app

import fasthttp
import net.websocket
import os
import time
import db.mysql

// HTTP Handler using fasthttp append_handler
pub fn create_http_handler(state &AppState) fasthttp.AppendHandler {
	return fn (req fasthttp.HttpRequest, mut out []u8, ws voidptr, mut ctl fasthttp.ResponseControl) fasthttp.Step {
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
pub fn create_websocket_server(mut state AppState, db mysql.DB, port int) &websocket.Server {
	mut ws_server := websocket.new_server(.ip, port, '/ws')
	
	ws_server.on_message(fn [mut state, db] (mut ws websocket.Client, msg &websocket.Message) ! {
		data := msg.payload.bytestr()
		if data == '' { return }
		
		client_ip := ws.conn.peer_ip() or { '127.0.0.1' }
		response := handle_ws_message(data, mut state, db, client_ip)
		
		if response.len > 0 {
			ws.write_string(response) or { return }
		}
	})
	
	ws_server.on_close(fn (mut ws websocket.Client, code int, reason string) ! {})
	
	return ws_server
}

fn handle_ws_message(msg string, mut state AppState, db mysql.DB, client_ip string) string {
	if msg == '{"type":"ping"}' || msg == 'ping' {
		return '{"type":"pong"}'
	}
	
	if msg.contains('"type":"check_wallet"') {
		if !check_rate_limit(mut state, client_ip) {
			return '{"type":"error","message":"Rate limit exceeded"}'
		}
		
		// Parse JSON - find "address":" then extract until next "
		if idx1 := msg.index('"address":"') {
			addr_start := idx1 + 11
			rest := msg[addr_start..]
			if idx2 := rest.index('"') {
				addr := rest[..idx2]
				result := check_wallet_from_db(db, addr)
				remaining := get_remaining(&state, client_ip)
				return '{"type":"wallet_result","wallet":${result},"remaining":${remaining}}'
			}
		}
		
		return '{"type":"error","message":"Invalid message format"}'
	}
	
	if msg == '{"type":"get_stats"}' || msg == 'get_stats' {
		stats := get_stats_from_db(db)
		return '{"type":"stats",${stats}}'
	}
	
	if msg == '{"type":"get_wallets"}' || msg == 'get_wallets' {
		wallets := get_wallets_from_db(db)
		return '{"type":"wallets","wallets":${wallets}}'
	}
	
	if msg == '{"type":"get_rate_limit"}' || msg == 'get_rate_limit' {
		remaining := get_remaining(&state, client_ip)
		return '{"type":"rate_limit","remaining":${remaining},"max":${state.config.max_checks_per_ip}}'
	}
	
	return ''
}

// ORM functions using db.mysql
fn check_wallet_from_db(db mysql.DB, address string) string {
	// Try to find wallet using ORM
	wallets := sql db {
		select from Wallet where address == address limit 1
	} or { []Wallet{} }
	
	if wallets.len > 0 {
		w := wallets[0]
		return '{"address":"${w.address}","status":"${w.status}","balance":${w.balance},"tokens":"${w.tokens}","source":"database"}'
	}
	
	// Not found - return random demo
	idx := address.len % demo_wallets.len
	w := demo_wallets[idx]
	return '{"address":"${address}","status":"${w.status}","balance":${w.balance},"tokens":"${w.tokens}","source":"generated"}'
}

fn get_stats_from_db(db mysql.DB) string {
	hacked := sql db {
		select count from Wallet where status == 'hacked'
	} or { 0 }
	vulnerable := sql db {
		select count from Wallet where status == 'vulnerable'
	} or { 0 }
	safe := sql db {
		select count from Wallet where status == 'safe'
	} or { 0 }
	total := sql db {
		select count from Wallet
	} or { 0 }
	
	return '{"hacked":${hacked},"vulnerable":${vulnerable},"safe":${safe},"total":${total},"total_checks":0}'
}

fn get_wallets_from_db(db mysql.DB) string {
	wallets := sql db {
		select from Wallet order by status
	} or { []Wallet{} }
	
	mut items := '['
	for i, w in wallets {
		if i > 0 { items += ',' }
		items += '{"address":"${w.address}","status":"${w.status}","balance":${w.balance},"tokens":"${w.tokens}"}'
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
