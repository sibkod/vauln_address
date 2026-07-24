module main

import fasthttp
import app
import mysql

fn main() {
	println('===========================================')
	println('  Wallet Checker Server')
	println('  V Language + FastHTTP + WebSocket + MySQL')
	println('===========================================')
	
	mut state := app.create_app_state()
	db_config := app.get_db_config()
	
	// Initialize database
	println('')
	println('Connecting to MySQL...')
	app.ensure_database(db_config) or {
		eprintln('Failed to ensure database: ${err}')
		return
	}
	
	// Connect to MySQL
	mut db := mysql.connect(mysql.Config{
		host: db_config.host
		port: db_config.port
		user: db_config.user
		password: db_config.password
		database: db_config.database
	}) or {
		eprintln('Failed to connect to MySQL: ${err}')
		return
	}
	println('Connected to MySQL!')
	
	// Run migrations
	mut migrator := app.new_migrator(&db)
	migrator.run() or {
		eprintln('Failed to run migrations: ${err}')
		return
	}
	
	// Seed demo data
	migrator.seed_demo_data() or {
		eprintln('Warning: Failed to seed demo data: ${err}')
	}
	
	// Start WebSocket server on port 8081
	mut ws_server := app.create_websocket_server(mut state, &db, state.config.ws_port)
	spawn ws_server.listen()
	
	// Start HTTP server with fasthttp on port 8080
	println('')
	println('HTTP Server:     http://localhost:${state.config.http_port}')
	println('WebSocket:      ws://localhost:${state.config.ws_port}/ws')
	println('Rate limit:     ${state.config.max_checks_per_ip} checks per IP per ${state.config.rate_limit_ttl}s')
	println('HTML:           ./templates/index.html')
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
