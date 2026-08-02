module main

import veb
import os
import json2 as json
import time
import db.sqlite

const port = 8080

pub struct Context {
	veb.Context
}

// Models
pub struct Wallet {
    id         int       @[primary; sql: serial]
    address    string    @[sql_type: 'VARCHAR(100)']
    chain      string    @[sql_type: 'VARCHAR(20)']
    status     string    @[sql_type: 'VARCHAR(20)']
    has_pk     bool      @[sql_type: 'TINYINT(1)']
    has_seed   bool      @[sql_type: 'TINYINT(1)']
    reason     string    @[sql_type: 'TEXT']
    source     string    @[sql_type: 'VARCHAR(100)']
    created_at time.Time @[sql_type: 'TIMESTAMP']
    updated_at time.Time @[sql_type: 'TIMESTAMP']
}

pub struct User {
    id             int        @[primary; sql: serial]
    wallet_address string     @[sql_type: 'VARCHAR(100)']
    chain          string     @[sql_type: 'VARCHAR(20)']
    nonce          string     @[sql_type: 'VARCHAR(100)']
    balance        int        @[sql_type: 'INT']
    is_premium     bool       @[sql_type: 'TINYINT(1)']
    created_at     time.Time  @[sql_type: 'TIMESTAMP']
    updated_at     time.Time  @[sql_type: 'TIMESTAMP']
    last_login_at  time.Time  @[sql_type: 'TIMESTAMP']
}

pub struct Order {
    id              int        @[primary; sql: serial]
    wallet_address  string     @[sql_type: 'VARCHAR(100)']
    chain           string     @[sql_type: 'VARCHAR(20)']
    order_uuid      string     @[sql_type: 'VARCHAR(100)']
    checks_count    int        @[sql_type: 'INT']
    total_usd       f64        @[sql_type: 'DECIMAL(10, 2)']
    currency        string     @[sql_type: 'VARCHAR(20)']
    token_amount    f64        @[sql_type: 'DECIMAL(20, 8)']
    payment_address string     @[sql_type: 'VARCHAR(200)']
    status          string     @[sql_type: 'VARCHAR(20)']
    tx_hash         string     @[sql_type: 'VARCHAR(200)']
    created_at      time.Time  @[sql_type: 'TIMESTAMP']
    updated_at      time.Time  @[sql_type: 'TIMESTAMP']
    completed_at    time.Time  @[sql_type: 'TIMESTAMP']
}

pub struct WalletCheckResponse {
    address       string
    chain         string
    status        string
    is_registered bool
    reason        string
}

pub struct AddWalletRequest {
    addresses   map[string]string
    status      string
    reason      string
    source      string
    seed_phrase string
}

pub struct AddWalletResponse {
    success         bool
    wallets_added  int
    wallets_skipped int
    message        string
}

pub struct DBConfig {
    db_path string
}

pub struct Database {
    conn sqlite.DB
}

pub fn connect_db(cfg DBConfig) !Database {
    mut conn := sqlite.connect(cfg.db_path)!
    return Database{conn: conn}
}

// close is not needed for SQLite as it auto-closes
// pub fn (db Database) close() { db.conn.close() }

pub fn (db Database) init_schema() {
    db.conn.exec_none('CREATE TABLE IF NOT EXISTS wallets (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        address TEXT NOT NULL,
        chain TEXT NOT NULL,
        status TEXT NOT NULL,
        has_pk INTEGER DEFAULT 0,
        has_seed INTEGER DEFAULT 0,
        reason TEXT,
        source TEXT,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        UNIQUE(chain, address)
    )')
    
    db.conn.exec_none('CREATE TABLE IF NOT EXISTS users (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        wallet_address TEXT NOT NULL,
        chain TEXT NOT NULL,
        nonce TEXT,
        balance INTEGER DEFAULT 10,
        is_premium INTEGER DEFAULT 0,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        last_login_at DATETIME,
        UNIQUE(wallet_address, chain)
    )')
    
    db.conn.exec_none('CREATE TABLE IF NOT EXISTS orders (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        wallet_address TEXT NOT NULL,
        chain TEXT NOT NULL,
        order_uuid TEXT UNIQUE NOT NULL,
        checks_count INTEGER NOT NULL,
        total_usd REAL NOT NULL,
        currency TEXT NOT NULL,
        token_amount REAL,
        payment_address TEXT,
        status TEXT DEFAULT "pending",
        tx_hash TEXT,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        completed_at DATETIME
    )')
    
    db.conn.exec_none('CREATE TABLE IF NOT EXISTS check_history (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        wallet_address TEXT DEFAULT "",
        address TEXT NOT NULL,
        chain TEXT NOT NULL,
        status TEXT,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    )')
}

pub fn (db Database) check_wallet(chain string, address string) !WalletCheckResponse {
    rows := db.conn.exec_param_many('SELECT address, chain, status, reason FROM wallets WHERE chain = ? AND address = ?', [chain, address])!
    
    if rows.len > 0 {
        row := rows[0]
        return WalletCheckResponse{
            address: row.vals[0].str()
            chain: row.vals[1].str()
            status: row.vals[2].str()
            is_registered: true
            reason: if row.vals.len > 3 { row.vals[3].str() } else { '' }
        }
    }
    
    return WalletCheckResponse{
        address: address
        chain: chain
        status: 'unknown'
        is_registered: false
    }
}

pub fn (db Database) add_wallet(address string, chain string, wallet_status string, reason string, source string) !AddWalletResponse {
    rows := db.conn.exec_param_many('SELECT id FROM wallets WHERE chain = ? AND address = ?', [chain, address])!
    
    if rows.len > 0 {
        return AddWalletResponse{
            success: false
            wallets_added: 0
            wallets_skipped: 1
            message: 'Wallet already exists'
        }
    }
    
    db.conn.exec_param_many('INSERT INTO wallets (address, chain, status, reason, source) VALUES (?, ?, ?, ?, ?)',
        [address, chain, wallet_status, reason, source]) or {}
    
    return AddWalletResponse{
        success: true
        wallets_added: 1
        wallets_skipped: 0
        message: 'Wallet added successfully'
    }
}

pub fn (db Database) get_user_or_create(wallet_address string, chain string) !User {
    rows := db.conn.exec_param_many('SELECT id, wallet_address, chain, balance, is_premium FROM users WHERE wallet_address = ? AND chain = ?', [wallet_address, chain])!
    
    if rows.len > 0 {
        row := rows[0]
        return User{
            id: row.vals[0].int()
            wallet_address: row.vals[1].str()
            chain: row.vals[2].str()
            balance: row.vals[3].int()
            is_premium: row.vals[4].int() == 1
        }
    }
    
    db.conn.exec_param_many('INSERT INTO users (wallet_address, chain, balance) VALUES (?, ?, 10)', [wallet_address, chain]) or {}
    
    return User{
        wallet_address: wallet_address
        chain: chain
        balance: 10
        is_premium: false
    }
}

pub fn (db Database) record_check(wallet_address string, address string, chain string, status string) {
    db.conn.exec_param_many('INSERT INTO check_history (wallet_address, address, chain, status) VALUES (?, ?, ?, ?)',
        [wallet_address, address, chain, status]) or {}
}

pub fn (db Database) get_wallet_stats() !map[string]int {
    rows := db.conn.exec('SELECT chain, COUNT(*) FROM wallets GROUP BY chain')!
    
    mut stats := map[string]int{}
    for row in rows {
        chain := row.vals[0].str()
        count := row.vals[1].int()
        stats[chain] = count
    }
    return stats
}

// Response structs
pub struct HealthResponse {
	status  string
	service string
}

pub struct ChainsResponse {
	chains []string
}

pub struct PricingResponse {
	price_per_check_usd f64
	solana_price_usd    f64
}

pub struct CheckWalletResponse {
	address       string
	chain         string
	status        string
	is_registered bool
	reason        string
}

pub struct AddWalletResponse2 {
	success         bool
	wallets_added  int
	wallets_skipped int
	messages        []string
}

pub struct CreateOrderResponse {
	order_uuid   string
	status       string
	checks_count int
}

pub struct ContactResponse {
	message string
	name    string
}

pub struct StatsResponse {
	evm     int
	btc     int
	solana  int
	sui     int
	tron    int
}

pub struct ErrorResponse {
	error   string
	details string
}

// App struct
struct App {
	veb.Middleware[Context]
mut:
	db Database
}

pub fn (ctx &Context) before_request() {}

@['/api/health'; get]
pub fn (mut app App) health(mut ctx Context) veb.Result {
	return ctx.json(HealthResponse{
		status: 'ok'
		service: 'vauln-address-api'
	})
}

@['/api/chains'; get]
pub fn (mut app App) chains(mut ctx Context) veb.Result {
	return ctx.json(ChainsResponse{
		chains: ['evm', 'btc', 'solana', 'sui', 'tron']
	})
}

@['/api/pricing'; get]
pub fn (mut app App) pricing(mut ctx Context) veb.Result {
	return ctx.json(PricingResponse{
		price_per_check_usd: 0.10
		solana_price_usd: 150.0
	})
}

@['/api/check/:chain/:address'; post]
pub fn (mut app App) check_wallet(mut ctx Context, chain string, address string) veb.Result {
	result := app.db.check_wallet(chain, address) or {
		return ctx.json(ErrorResponse{
			error: 'Database error'
			details: err.msg()
		})
	}

	return ctx.json(CheckWalletResponse{
		address: result.address
		chain: result.chain
		status: result.status
		is_registered: result.is_registered
		reason: result.reason
	})
}

@['/api/wallets'; post]
pub fn (mut app App) add_wallet(mut ctx Context) veb.Result {
	body := ctx.req.data
	req := json.decode[AddWalletRequest](body) or {
		return ctx.json(ErrorResponse{
			error: 'Invalid request body'
			details: err.msg()
		})
	}

	mut wallets_added := 0
	mut wallets_skipped := 0
	mut messages := []string{}

	for chain, address in req.addresses {
		result := app.db.add_wallet(address, chain, req.status, req.reason, req.source) or {
			messages << 'Error adding ${chain}/${address}: ${err.msg()}'
			wallets_skipped++
			continue
		}

		if result.success {
			wallets_added++
			messages << 'Added ${chain}/${address}'
		} else {
			wallets_skipped++
			messages << result.message + ': ${chain}/${address}'
		}
	}

	success := wallets_added > 0

	return ctx.json(AddWalletResponse2{
		success: success
		wallets_added: wallets_added
		wallets_skipped: wallets_skipped
		messages: messages
	})
}

@['/api/stats'; get]
pub fn (mut app App) stats(mut ctx Context) veb.Result {
	db_stats := app.db.get_wallet_stats() or {
		return ctx.json(ErrorResponse{
			error: 'Failed to get stats'
			details: err.msg()
		})
	}

	return ctx.json(StatsResponse{
		evm: db_stats['evm'] or { 0 }
		btc: db_stats['btc'] or { 0 }
		solana: db_stats['solana'] or { 0 }
		sui: db_stats['sui'] or { 0 }
		tron: db_stats['tron'] or { 0 }
	})
}

pub struct OrderReq {
	chain       string
	address     string
	checks_count int
}

pub struct ContactReq {
	name    string
	email   string
	message string
}

@['/api/order'; post]
pub fn (mut app App) create_order(mut ctx Context) veb.Result {
	body := ctx.req.data

	req := json.decode[OrderReq](body) or {
		return ctx.json(ErrorResponse{
			error: 'Invalid request body'
			details: err.msg()
		})
	}

	return ctx.json(CreateOrderResponse{
		order_uuid: 'order_12345678'
		status: 'pending'
		checks_count: req.checks_count
	})
}

@['/contact'; post]
pub fn (mut app App) contact(mut ctx Context) veb.Result {
	body := ctx.req.data
	req := json.decode[ContactReq](body) or {
		return ctx.json(ErrorResponse{
			error: 'Invalid request body'
			details: 'Failed to parse request'
		})
	}

	return ctx.json(ContactResponse{
		message: 'ok'
		name: req.name
	})
}

fn main() {
	// Get database configuration from environment
	db_path := os.getenv('DB_PATH')

	// Default values for development
	db_file := if db_path == '' { 'vauln_address.db' } else { db_path }

	println('Starting VaulnAddress API on port ${port}...')
	println('Connecting to SQLite database at ${db_file}...')

	db_cfg := DBConfig{
		db_path: db_file
	}

	db := connect_db(db_cfg) or {
		println('Failed to connect to database: ${err.msg()}')
		println('Starting in demo mode without database...')
		mut app := &App{}
		veb.run[App, Context](mut app, port)
		return
	}

	// Initialize schema
	db.init_schema()

	mut app := &App{}
	app.db = db
	veb.run[App, Context](mut app, port)
}
