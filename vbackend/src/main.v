module main

import veb
import os
import json2 as json
import time
import db.sqlite
import rand

const port = 8080

pub struct Context {
	veb.Context
}

// ==================== Constants ====================

const valid_chains = ['evm', 'btc', 'solana', 'sui', 'tron']
const valid_statuses = ['hacked', 'vulnerable', 'safe', 'hacker', 'drained']

fn is_valid_chain(chain string) bool {
	for c in valid_chains {
		if c == chain {
			return true
		}
	}
	return false
}

fn is_valid_status(status string) bool {
	for s in valid_statuses {
		if s == status {
			return true
		}
	}
	return false
}

// ==================== Database Models ====================

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
    
    db.conn.exec_none('CREATE TABLE IF NOT EXISTS api_keys (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        wallet_address TEXT NOT NULL,
        key_hash TEXT NOT NULL,
        key_prefix TEXT NOT NULL,
        name TEXT NOT NULL,
        last_used_at DATETIME,
        expires_at DATETIME,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        revoked_at DATETIME,
        is_revoked INTEGER DEFAULT 0
    )')
}

// ==================== Database Operations ====================

pub fn (db Database) get_user_by_address(wallet_address string, chain string) !User {
    rows := db.conn.exec_param_many('SELECT id, wallet_address, chain, balance, is_premium FROM users WHERE wallet_address = ? AND chain = ?', [wallet_address, chain])!
    
    if rows.len == 0 {
        return error('User not found')
    }
    
    row := rows[0]
    return User{
        id: row.vals[0].int()
        wallet_address: row.vals[1].str()
        chain: row.vals[2].str()
        balance: row.vals[3].int()
        is_premium: row.vals[4].int() == 1
    }
}

pub fn (db Database) create_user(wallet_address string, chain string) !User {
    db.conn.exec_param_many('INSERT INTO users (wallet_address, chain, balance) VALUES (?, ?, 10)', [wallet_address, chain]) or {}
    
    return User{
        wallet_address: wallet_address
        chain: chain
        balance: 10
        is_premium: false
    }
}

pub fn (db Database) get_or_create_user(wallet_address string, chain string) !User {
    user := db.get_user_by_address(wallet_address, chain) or {
        return db.create_user(wallet_address, chain)
    }
    return user
}

pub fn (db Database) deduct_balance(wallet_address string, chain string, amount int) !bool {
    result := db.conn.exec_param_many('UPDATE users SET balance = balance - ? WHERE wallet_address = ? AND chain = ? AND balance >= ?', [wallet_address.str(), chain, amount.str()])!
    return result.len > 0
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
        status: 'not_found'
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

pub fn (db Database) record_check(wallet_address string, address string, chain string, status string) {
    db.conn.exec_param_many('INSERT INTO check_history (wallet_address, address, chain, status) VALUES (?, ?, ?, ?)',
        [wallet_address, address, chain, status]) or {}
}

pub fn (db Database) get_check_history(wallet_address string, limit int) ![]RecentCheck {
    rows := db.conn.exec_param_many('SELECT id, address, chain, status, created_at FROM check_history WHERE wallet_address = ? ORDER BY created_at DESC LIMIT ?', [wallet_address, limit.str()])!
    
    mut checks := []RecentCheck{}
    for row in rows {
        checks << RecentCheck{
            id: row.vals[0].int()
            address: row.vals[1].str()
            chain: row.vals[2].str()
            status: row.vals[3].str()
            checked_at: row.vals[4].str()
        }
    }
    return checks
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

pub fn (db Database) create_order(wallet_address string, chain string, checks_count int, currency string) !Order {
    price_per_check := 0.10
    total_usd := f64(checks_count) * price_per_check
    
    order_uuid := 'order_${rand.uuid_v4()}'
    payment_address := 'DemoPaymentAddress123'
    
    db.conn.exec_param_many('INSERT INTO orders (wallet_address, chain, order_uuid, checks_count, total_usd, currency, token_amount, payment_address, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)',
        [wallet_address, chain, order_uuid, checks_count.str(), total_usd.str(), currency, total_usd.str(), payment_address, 'pending']) or {}
    
    return Order{
        order_uuid: order_uuid
        wallet_address: wallet_address
        chain: chain
        checks_count: checks_count
        total_usd: total_usd
        currency: currency
        token_amount: total_usd
        payment_address: payment_address
        status: 'pending'
    }
}

pub fn (db Database) get_order(order_uuid string) !Order {
    rows := db.conn.exec_param_many('SELECT order_uuid, wallet_address, chain, checks_count, total_usd, currency, token_amount, payment_address, status FROM orders WHERE order_uuid = ?', [order_uuid])!
    
    if rows.len == 0 {
        return error('Order not found')
    }
    
    row := rows[0]
    return Order{
        order_uuid: row.vals[0].str()
        wallet_address: row.vals[1].str()
        chain: row.vals[2].str()
        checks_count: row.vals[3].int()
        total_usd: row.vals[4].f64()
        currency: row.vals[5].str()
        token_amount: row.vals[6].f64()
        payment_address: row.vals[7].str()
        status: row.vals[8].str()
    }
}

pub fn (db Database) update_order_status(order_uuid string, status string, tx_hash string) ! {
    db.conn.exec_param_many('UPDATE orders SET status = ?, tx_hash = ? WHERE order_uuid = ?', [status, tx_hash, order_uuid]) or {}
}

pub fn (db Database) get_user_orders(wallet_address string) ![]Order {
    rows := db.conn.exec_param_many('SELECT order_uuid, wallet_address, chain, checks_count, total_usd, currency, token_amount, payment_address, status FROM orders WHERE wallet_address = ? ORDER BY created_at DESC', [wallet_address])!
    
    mut orders := []Order{}
    for row in rows {
        orders << Order{
            order_uuid: row.vals[0].str()
            wallet_address: row.vals[1].str()
            chain: row.vals[2].str()
            checks_count: row.vals[3].int()
            total_usd: row.vals[4].f64()
            currency: row.vals[5].str()
            token_amount: row.vals[6].f64()
            payment_address: row.vals[7].str()
            status: row.vals[8].str()
        }
    }
    return orders
}

// ==================== Request/Response Structs ====================

pub struct User {
    id             int
    wallet_address string
    chain          string
    balance        int
    is_premium     bool
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

pub struct RecentCheck {
    id         int
    address    string
    chain      string
    status     string
    checked_at string
}

pub struct Order {
    order_uuid       string
    wallet_address   string
    chain            string
    checks_count     int
    total_usd        f64
    currency         string
    token_amount     f64
    payment_address  string
    status           string
}

// ==================== API Request/Response Structs ====================

pub struct NonceRequest {
    address string
    chain   string
}

pub struct NonceResponse {
    nonce   string
    message string
}

pub struct AuthRequest {
    address  string
    chain    string
    signature string
    message  string
}

pub struct AuthResponse {
    token     string
    user      UserPublic
    expires_in int
}

pub struct UserPublic {
    wallet_address string
    chain         string
    balance       int
    is_premium    bool
}

pub struct CheckRequest {
    address string
    chain   string
}

pub struct CheckResponse {
    address      string
    chain        string
    status       string
    has_pk       bool
    has_seed     bool
    found        bool
    balance_left int
}

pub struct CreateOrderRequest {
    chain        string
    wallet_address string
    checks       int
}

pub struct CreateOrderResponse {
    order_id        string
    checks_count    int
    total_usd      f64
    amount         string
    payment_address string
    status         string
}

pub struct PurchaseHistoryResponse {
    orders []Order
}

pub struct CancelOrderRequest {
    order_id string
}

pub struct ConfirmOrderRequest {
    order_id string
    tx_hash  string
}

pub struct VerifyPaymentRequest {
    tx_hash string
}

pub struct GetPaymentStatusRequest {
    signature string
}

pub struct PaymentStatusResponse {
    status    string
    tx_hash   string
    confirmed bool
}

pub struct CreateAPIKeyRequest {
    name      string
    expires_in int
}

pub struct APIKey {
    id             int
    wallet_address string
    key_prefix     string
    name           string
    last_used_at   string
    expires_at     string
    created_at     string
    is_revoked     bool
}

pub struct APIKeyResponse {
    key       string
    key_prefix string
    name      string
    expires_at string
    created_at string
}

pub struct ListAPIKeysResponse {
    keys []APIKey
}

pub struct RenewAPIKeyRequest {
    address   string
    chain     string
    signature string
    message   string
    key_id    int
}

// ==================== Generic Responses ====================

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

pub struct PackagesResponse {
    packages []Package
}

pub struct Package {
    checks_included  int
    price_usd       f64
    token_symbol    string
    has_discount    bool
    discount_label  string
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
    code    string
    details string
}

pub struct SuccessResponse {
    message string
}

pub struct ContactRequest {
    name    string
    email   string
    message string
}

pub struct ContactResponse {
    message string
    name    string
}

// ==================== App ====================

struct App {
    veb.Middleware[Context]
mut:
    db Database
}

pub fn (ctx &Context) before_request() {}

// ==================== Health & Info ====================

@['/api/health'; get]
pub fn (mut app App) health(mut ctx Context) veb.Result {
    return ctx.json(HealthResponse{
        status: 'ok'
        service: 'vauln-address-api'
    })
}

@['/api/chains'; get]
pub fn (mut app App) get_chains(mut ctx Context) veb.Result {
    return ctx.json(ChainsResponse{
        chains: valid_chains
    })
}

@['/api/pricing'; get]
pub fn (mut app App) get_pricing(mut ctx Context) veb.Result {
    return ctx.json(PricingResponse{
        price_per_check_usd: 0.10
        solana_price_usd: 150.0
    })
}

@['/api/packages'; get]
pub fn (mut app App) get_packages(mut ctx Context) veb.Result {
    return ctx.json(PackagesResponse{
        packages: [
            Package{checks_included: 10, price_usd: 1.0, token_symbol: 'USDC', has_discount: false, discount_label: ''},
            Package{checks_included: 50, price_usd: 4.0, token_symbol: 'USDC', has_discount: false, discount_label: ''},
            Package{checks_included: 100, price_usd: 7.0, token_symbol: 'USDC', has_discount: true, discount_label: '30% OFF'},
            Package{checks_included: 500, price_usd: 25.0, token_symbol: 'USDC', has_discount: true, discount_label: '50% OFF'},
        ]
    })
}

@['/api/stats'; get]
pub fn (mut app App) get_stats(mut ctx Context) veb.Result {
    db_stats := app.db.get_wallet_stats() or {
        return ctx.json(ErrorResponse{
            error: 'Failed to get stats'
            code: 'DB_ERROR'
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

// ==================== Authentication ====================

@['/api/auth/nonce'; get]
pub fn (mut app App) get_nonce(mut ctx Context) veb.Result {
    address := ctx.query['address'] or { '' }
    chain := ctx.query['chain'] or { '' }

    if address == '' || chain == '' {
        return ctx.json(ErrorResponse{
            error: 'address and chain are required'
            code: 'INVALID_REQUEST'
        })
    }

    if !is_valid_chain(chain) {
        return ctx.json(ErrorResponse{
            error: 'unsupported chain'
            code: 'INVALID_CHAIN'
        })
    }

    nonce := rand.string(32)
    now := time.now()
    timestamp := now.unix()
    
    return ctx.json(NonceResponse{
        nonce: nonce
        message: 'Sign this message to authenticate with Vauln Address.\n\nNonce: ${nonce}\nTimestamp: ${timestamp}'
    })
}

@['/api/auth/login'; post]
pub fn (mut app App) authenticate(mut ctx Context) veb.Result {
    body := ctx.req.data
    req := json.decode[AuthRequest](body) or {
        return ctx.json(ErrorResponse{
            error: 'invalid request body'
            code: 'INVALID_REQUEST'
            details: err.msg()
        })
    }

    // Demo: Create/get user and return token
    user := app.db.get_or_create_user(req.address, req.chain) or {
        return ctx.json(ErrorResponse{
            error: 'authentication failed'
            code: 'AUTH_FAILED'
        })
    }

    // Demo token (in production, use proper JWT)
    token := 'demo_token_${rand.string(32)}'

    return ctx.json(AuthResponse{
        token: token
        user: UserPublic{
            wallet_address: user.wallet_address
            chain: user.chain
            balance: user.balance
            is_premium: user.is_premium
        }
        expires_in: 86400
    })
}

@['/api/me'; get]
pub fn (mut app App) get_me(mut ctx Context) veb.Result {
    // Demo implementation
    return ctx.json(UserPublic{
        wallet_address: 'demo_wallet'
        chain: 'evm'
        balance: 10
        is_premium: false
    })
}

@['/api/user/profile'; get]
pub fn (mut app App) get_user_profile(mut ctx Context) veb.Result {
    return ctx.json(UserPublic{
        wallet_address: 'demo_wallet'
        chain: 'evm'
        balance: 10
        is_premium: false
    })
}

@['/api/user/balance'; get]
pub fn (mut app App) get_balance(mut ctx Context) veb.Result {
    return ctx.json({
        'balance': 10
        'rate_limit': 100
    })
}

@['/api/user/purchases'; get]
pub fn (mut app App) get_purchases(mut ctx Context) veb.Result {
    return ctx.json(PurchaseHistoryResponse{
        orders: []
    })
}

// ==================== Wallet Check ====================

@['/api/check'; post]
pub fn (mut app App) check_wallet(mut ctx Context) veb.Result {
    body := ctx.req.data
    req := json.decode[CheckRequest](body) or {
        return ctx.json(ErrorResponse{
            error: 'invalid request body'
            code: 'INVALID_REQUEST'
        })
    }

    if !is_valid_chain(req.chain) {
        return ctx.json(ErrorResponse{
            error: 'unsupported chain'
            code: 'INVALID_CHAIN'
            details: 'supported chains: evm, btc, solana, sui, tron'
        })
    }

    result := app.db.check_wallet(req.chain, req.address) or {
        return ctx.json(ErrorResponse{
            error: 'database error'
            code: 'DB_ERROR'
        })
    }

    return ctx.json(CheckResponse{
        address: result.address
        chain: result.chain
        status: result.status
        has_pk: false
        has_seed: false
        found: result.is_registered
        balance_left: 9
    })
}

pub struct CheckHistoryResponse {
    checks []RecentCheck
    total  int
}

@['/api/recent'; get]
pub fn (mut app App) get_recent_checks(mut ctx Context) veb.Result {
    limit_str := ctx.query['limit'] or { '10' }
    mut limit := limit_str.int()
    if limit > 100 { limit = 100 }
    if limit <= 0 { limit = 10 }

    checks := app.db.get_check_history('', limit) or {
        return ctx.json(ErrorResponse{
            error: 'database error'
            code: 'DB_ERROR'
        })
    }

    return ctx.json(CheckHistoryResponse{
        checks: checks
        total: checks.len
    })
}

// ==================== Admin: Add Wallet ====================

@['/api/wallets'; post]
pub fn (mut app App) add_wallet(mut ctx Context) veb.Result {
    body := ctx.req.data
    req := json.decode[AddWalletRequest](body) or {
        return ctx.json(ErrorResponse{
            error: 'Invalid request body'
            code: 'INVALID_REQUEST'
            details: err.msg()
        })
    }

    if !is_valid_status(req.status) {
        return ctx.json(ErrorResponse{
            error: 'invalid status'
            code: 'INVALID_STATUS'
            details: 'valid statuses: hacked, vulnerable, safe, hacker, drained'
        })
    }

    mut wallets_added := 0
    mut wallets_skipped := 0
    mut skipped_wallets := []SkippedWallet{}

    for chain, address in req.addresses {
        if !is_valid_chain(chain) {
            skipped_wallets << SkippedWallet{
                address: address
                chain: chain
                reason: 'invalid chain'
            }
            wallets_skipped++
            continue
        }

        result := app.db.add_wallet(address, chain, req.status, req.reason, req.source) or {
            skipped_wallets << SkippedWallet{
                address: address
                chain: chain
                reason: 'database error'
            }
            wallets_skipped++
            continue
        }

        if result.success {
            wallets_added++
        } else {
            wallets_skipped++
            skipped_wallets << SkippedWallet{
                address: address
                chain: chain
                reason: 'duplicate wallet'
            }
        }
    }

    return ctx.json(AddWalletResponseFull{
        success: wallets_added > 0
        wallets_added: wallets_added
        wallets_skipped: wallets_skipped
        wallet_ids: []
        skipped_wallets: skipped_wallets
        message: if wallets_added > 0 { 'Wallets added successfully' } else { 'No wallets added' }
    })
}

pub struct SkippedWallet {
    address string
    chain   string
    reason  string
}

pub struct AddWalletResponseFull {
    success         bool
    wallets_added  int
    wallets_skipped int
    wallet_ids      []int
    skipped_wallets []SkippedWallet
    message         string
}

// ==================== Orders ====================

@['/api/orders'; post]
pub fn (mut app App) create_order(mut ctx Context) veb.Result {
    body := ctx.req.data
    req := json.decode[CreateOrderRequest](body) or {
        return ctx.json(ErrorResponse{
            error: 'Invalid request body'
            code: 'INVALID_REQUEST'
        })
    }

    order := app.db.create_order(req.wallet_address, req.chain, req.checks, 'usdc') or {
        return ctx.json(ErrorResponse{
            error: 'Failed to create order'
            code: 'DB_ERROR'
        })
    }

    return ctx.json(CreateOrderResponse{
        order_id: order.order_uuid
        checks_count: order.checks_count
        total_usd: order.total_usd
        amount: order.token_amount.str()
        payment_address: order.payment_address
        status: order.status
    })
}

@['/api/orders/:id/cancel'; post]
pub fn (mut app App) cancel_order(mut ctx Context, id string) veb.Result {
    app.db.update_order_status(id, 'cancelled', '') or {}
    
    return ctx.json(SuccessResponse{
        message: 'Order cancelled'
    })
}

@['/api/orders/:id/confirm'; post]
pub fn (mut app App) confirm_order(mut ctx Context, id string) veb.Result {
    body := ctx.req.data
    req := json.decode[ConfirmOrderRequest](body) or {
        return ctx.json(ErrorResponse{
            error: 'Invalid request body'
            code: 'INVALID_REQUEST'
        })
    }

    app.db.update_order_status(id, 'completed', req.tx_hash) or {}
    
    return ctx.json(SuccessResponse{
        message: 'Order confirmed'
    })
}

@['/api/orders/verify'; get]
pub fn (mut app App) verify_payment(mut ctx Context) veb.Result {
    tx_hash := ctx.query['tx_hash'] or { '' }
    
    return ctx.json(PaymentStatusResponse{
        status: 'pending'
        tx_hash: tx_hash
        confirmed: false
    })
}

@['/api/payment/status/:signature'; post]
pub fn (mut app App) get_payment_status(mut ctx Context, signature string) veb.Result {
    return ctx.json(PaymentStatusResponse{
        status: 'pending'
        tx_hash: signature
        confirmed: false
    })
}

// ==================== API Keys ====================

@['/api/api-keys'; get]
pub fn (mut app App) list_api_keys(mut ctx Context) veb.Result {
    return ctx.json(ListAPIKeysResponse{
        keys: []
    })
}

@['/api/api-keys'; post]
pub fn (mut app App) create_api_key(mut ctx Context) veb.Result {
    body := ctx.req.data
    req := json.decode[CreateAPIKeyRequest](body) or {
        return ctx.json(ErrorResponse{
            error: 'Invalid request body'
            code: 'INVALID_REQUEST'
        })
    }

    api_key := 'vkn_${rand.string(32)}'
    key_prefix := api_key[..8]

    return ctx.json(APIKeyResponse{
        key: api_key
        key_prefix: key_prefix
        name: req.name
        expires_at: ''
        created_at: time.now().format()
    })
}

@['/api/api-keys/:id'; delete]
pub fn (mut app App) delete_api_key(mut ctx Context, id string) veb.Result {
    return ctx.json(SuccessResponse{
        message: 'API key deleted'
    })
}

@['/api/api-keys/revoke/:id'; post]
pub fn (mut app App) revoke_api_key(mut ctx Context, id string) veb.Result {
    return ctx.json(SuccessResponse{
        message: 'API key revoked'
    })
}

@['/api/api-keys/renew'; post]
pub fn (mut app App) renew_api_key(mut ctx Context) veb.Result {
    body := ctx.req.data
    _ := json.decode[RenewAPIKeyRequest](body) or {
        return ctx.json(ErrorResponse{
            error: 'Invalid request body'
            code: 'INVALID_REQUEST'
        })
    }

    return ctx.json(SuccessResponse{
        message: 'API key renewed'
    })
}

// ==================== Contact ====================

@['/contact'; post]
pub fn (mut app App) contact(mut ctx Context) veb.Result {
    body := ctx.req.data
    req := json.decode[ContactRequest](body) or {
        return ctx.json(ErrorResponse{
            error: 'Invalid request body'
            code: 'INVALID_REQUEST'
        })
    }

    return ctx.json(ContactResponse{
        message: 'ok'
        name: req.name
    })
}

// ==================== Main ====================

fn main() {
    db_path := os.getenv('DB_PATH')
    db_file := if db_path == '' { 'vauln_address.db' } else { db_path }

    println('Starting VaulnAddress API on port ${port}...')
    println('Connecting to SQLite database at ${db_file}...')

    db_cfg := DBConfig{
        db_path: db_file
    }

    db := connect_db(db_cfg) or {
        println('Failed to connect to database: ${err.msg()}')
        mut app := &App{}
        veb.run[App, Context](mut app, port)
        return
    }

    db.init_schema()

    mut app := &App{}
    app.db = db
    veb.run[App, Context](mut app, port)
}
