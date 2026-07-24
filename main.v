module main

import fasthttp
import app

fn main() {
	println('===========================================')
	println('  Wallet Checker Server')
	println('  V Language + FastHTTP + WebSocket + MySQL ORM')
	println('===========================================')
	
	mut state := app.create_app_state()
	db_config := app.get_db_config()
	
	// Initialize database
	println('')
	println('Connecting to MySQL...')
	app.init_database(db_config) or {
		eprintln('Failed to init database: ${err}')
		return
	}
	
	// Connect to MySQL
	db := app.db_connect(db_config) or {
		eprintln('Failed to connect to MySQL: ${err}')
		return
	}
	println('Connected to MySQL!')
	
	// Start WebSocket server
	mut ws_server := app.create_websocket_server(mut state, db, state.config.ws_port)
	spawn ws_server.listen()
	
	// Start HTTP server
	println('')
	println('HTTP Server:     http://localhost:${state.config.http_port}')
	println('WebSocket:      ws://localhost:${state.config.ws_port}/ws')
	println('Rate limit:     ${state.config.max_checks_per_ip} checks per IP per ${state.config.rate_limit_ttl}s')
	println('')
	println('Press Ctrl+C to stop')
	
	handler := app.create_http_handler(state)
	
	mut server := fasthttp.new_server(fasthttp.ServerConfig{
		port: state.config.http_port
		append_handler: handler
	}) or {
		eprintln('Failed to create server: ${err}')
		return
	}
	
	server.run() or { eprintln('Server error: ${err}') }
}
