module main

import fasthttp
import app

fn main() {
	println('===========================================')
	println('  Wallet Checker Server')
	println('  V Language + FastHTTP + WebSocket')
	println('===========================================')
	
	mut state := app.create_app_state()
	
	// Start WebSocket server on port 8081
	mut ws_server := app.create_websocket_server(mut state, state.config.ws_port)
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
