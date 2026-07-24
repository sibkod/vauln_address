-- Migration: 001_init.sql
-- Create tables for vauln-address API

-- Wallets table
CREATE TABLE IF NOT EXISTS wallets (
    id BIGSERIAL PRIMARY KEY,
    address VARCHAR(100) NOT NULL,
    chain VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL,
    has_pk BOOLEAN DEFAULT FALSE,
    has_seed BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for wallets
CREATE INDEX IF NOT EXISTS idx_wallets_address ON wallets(address);
CREATE INDEX IF NOT EXISTS idx_wallets_chain ON wallets(chain);
CREATE INDEX IF NOT EXISTS idx_wallets_status ON wallets(status);
CREATE INDEX IF NOT EXISTS idx_wallets_chain_status ON wallets(chain, status);

-- Users table (Web3 authenticated)
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    wallet_address VARCHAR(100) NOT NULL,
    chain VARCHAR(20) NOT NULL,
    nonce VARCHAR(100),
    balance INTEGER DEFAULT 10,
    is_premium BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_login_at TIMESTAMP WITH TIME ZONE,
    UNIQUE(wallet_address, chain)
);

CREATE INDEX IF NOT EXISTS idx_users_wallet ON users(wallet_address);

-- Orders table
CREATE TABLE IF NOT EXISTS orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id),
    order_uuid VARCHAR(100) UNIQUE NOT NULL,
    checks_count INTEGER NOT NULL,
    total_usd DECIMAL(10, 2) NOT NULL,
    currency VARCHAR(20) NOT NULL,
    token_amount DECIMAL(20, 8),
    payment_address VARCHAR(200),
    status VARCHAR(20) DEFAULT 'pending',
    tx_hash VARCHAR(200),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(user_id);
CREATE INDEX IF NOT EXISTS idx_orders_uuid ON orders(order_uuid);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);

-- Contact messages table
CREATE TABLE IF NOT EXISTS contact_messages (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Rate limits table
CREATE TABLE IF NOT EXISTS rate_limits (
    id BIGSERIAL PRIMARY KEY,
    ip_address VARCHAR(45) NOT NULL UNIQUE,
    request_count INTEGER DEFAULT 0,
    window_start TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rate_limits_ip ON rate_limits(ip_address);

-- Check history table (for analytics)
CREATE TABLE IF NOT EXISTS check_history (
    id BIGSERIAL PRIMARY KEY,
    address VARCHAR(100) NOT NULL,
    chain VARCHAR(20) NOT NULL,
    status VARCHAR(20),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_check_history_created ON check_history(created_at);

-- ============================================
-- DEMO DATA
-- ============================================

-- Hacked wallets
INSERT INTO wallets (address, chain, status, has_pk, has_seed, created_at) VALUES
('0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1', 'evm', 'hacked', true, false, NOW() - INTERVAL '1 day'),
('0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045', 'evm', 'hacked', true, false, NOW() - INTERVAL '2 days'),
('0xBE0eB53F46cd790Cd13851d5EFf43D12404d33E8', 'evm', 'hacked', false, true, NOW() - INTERVAL '3 days'),
('0x28C6c06298d514Db089934071355E5743bf21d60', 'evm', 'hacked', true, false, NOW() - INTERVAL '4 hours'),
('0x3fC91A3afd70395Cd223C05a797668C81272B4D2', 'evm', 'hacked', true, true, NOW() - INTERVAL '12 hours'),
('bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh', 'btc', 'hacked', true, false, NOW() - INTERVAL '5 hours'),
('bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx', 'btc', 'hacked', true, true, NOW() - INTERVAL '6 hours'),
('1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2', 'btc', 'hacked', true, false, NOW() - INTERVAL '1 day'),
('3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy', 'btc', 'hacked', false, true, NOW() - INTERVAL '2 days'),
('7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV', 'solana', 'hacked', true, false, NOW() - INTERVAL '3 hours'),
('5WvFHrQv7nC8E9fDx3mZ9yK4pR6jT2uX1bNcD8fGhJk', 'solana', 'hacked', true, true, NOW() - INTERVAL '8 hours'),
('TJK5M5kKxP8xF9cGvN2pL6rU4sW7xA3bCd', 'tron', 'hacked', true, false, NOW() - INTERVAL '2 hours'),
('TJygL3D2K8M7fGhJkLmNpQrStUvWxYzAbCd', 'tron', 'hacked', true, true, NOW() - INTERVAL '7 hours'),
('0x8a1c4cd2d2fd05e02e3d2b5f4d6f8a1c3e5b7d9f0a2c4e6', 'sui', 'hacked', true, false, NOW() - INTERVAL '10 hours'),
('0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef', 'sui', 'hacked', true, true, NOW() - INTERVAL '1 day');

-- Vulnerable wallets
INSERT INTO wallets (address, chain, status, has_pk, has_seed, created_at) VALUES
('0xAb5801a7D398351b8bE11C439e05C5B3259aeC9B', 'evm', 'vulnerable', false, false, NOW() - INTERVAL '1 day'),
('0x00000000219ab540356cBB839Cbe05303d7705Fa', 'evm', 'vulnerable', false, false, NOW() - INTERVAL '5 hours'),
('0x71C7656EC7ab88b098defB751B7401B5f6d8976F', 'evm', 'vulnerable', false, false, NOW() - INTERVAL '2 days'),
('bc1qnq0sr8ere5ppzksj0f7qepqknmu9n6nxfdxhv3', 'btc', 'vulnerable', false, false, NOW() - INTERVAL '6 hours'),
('1P7f4P8b9cK2mN5oR6sT1uV2wX3yZ4aB5cD6eF7g', 'btc', 'vulnerable', false, false, NOW() - INTERVAL '3 hours'),
('Gf5YB8nQ4rK7mX1vT2zP6jS9cL3oU5aW8eN1qR4tY', 'solana', 'vulnerable', false, false, NOW() - INTERVAL '4 hours'),
('TRx7R8yJ4kP2mN6oQ8sU1vW3xY5zA7bC9dE0fG', 'tron', 'vulnerable', false, false, NOW() - INTERVAL '2 hours'),
('0xAABBCCddEEFF0011223344556677889900aAbBcCdD', 'sui', 'vulnerable', false, false, NOW() - INTERVAL '1 hour');

-- Hacker wallets (known hacker addresses)
INSERT INTO wallets (address, chain, status, has_pk, has_seed, created_at) VALUES
('0x000000000000000000000000000000000000dEaD', 'evm', 'hacker', true, false, NOW() - INTERVAL '10 days'),
('0x0000000000000000000000000000000000000000', 'evm', 'hacker', true, false, NOW() - INTERVAL '20 days'),
('1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2', 'btc', 'hacker', true, false, NOW() - INTERVAL '15 days'),
('HackerSolana1234567890ABCDEFGHiJKLMNOP', 'solana', 'hacker', true, false, NOW() - INTERVAL '5 days'),
('TronHackerAddr123456789ABCDEFGHijkL', 'tron', 'hacker', true, false, NOW() - INTERVAL '7 days'),
('0xDEADBEEF1234567890ABCDEF1234567890ABCDEF', 'sui', 'hacker', true, false, NOW() - INTERVAL '3 days');

-- Drained wallets
INSERT INTO wallets (address, chain, status, has_pk, has_seed, created_at) VALUES
('0x1234567890AbCdEf1234567890aBcDeF12345678', 'evm', 'drained', true, false, NOW() - INTERVAL '8 hours'),
('0xAaBbCcDdEeFfAaBbCcDdEeFfAaBbCcDdEeFfAa', 'evm', 'drained', true, true, NOW() - INTERVAL '1 day'),
('bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq', 'btc', 'drained', true, true, NOW() - INTERVAL '2 days'),
('DrainedSolanaAddr1234567890ABCDEFGHijK', 'solana', 'drained', true, false, NOW() - INTERVAL '12 hours'),
('TRonDrain3dAddr123456789ABCDEFabcdef', 'tron', 'drained', true, false, NOW() - INTERVAL '6 hours');

-- Safe wallets (for contrast/testing)
INSERT INTO wallets (address, chain, status, has_pk, has_seed, created_at) VALUES
('0x4B0897b0513fdC7C541B6d9D7E929C4e5364D2dB', 'evm', 'safe', false, false, NOW() - INTERVAL '30 days'),
('bc1qeg0aq4m4ts3606wjr4g8rkn36xq4r7y5x6y7z', 'btc', 'safe', false, false, NOW() - INTERVAL '25 days'),
('SafeSolanaWallet1234567890ABCDEFGHijKL', 'solana', 'safe', false, false, NOW() - INTERVAL '20 days'),
('SafeTronWallet123456789ABCDEFabcdef', 'tron', 'safe', false, false, NOW() - INTERVAL '15 days'),
('0x0000000000000000000000000000000000000001', 'sui', 'safe', false, false, NOW() - INTERVAL '10 days');
