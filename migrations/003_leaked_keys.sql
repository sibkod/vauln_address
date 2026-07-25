-- Migration: 003_leaked_keys.sql
-- Table for storing leaked private keys and seed phrases
-- If leak_id is 0, it means the wallet was flagged by other triggers (not by seed/private key)

-- Leaked keys table
CREATE TABLE IF NOT EXISTS leaked_keys (
    id BIGSERIAL PRIMARY KEY,
    wallet_id BIGINT NOT NULL,                          -- Reference to wallets.id (0 = no seed/key found)
    address VARCHAR(200) NOT NULL,                       -- Wallet address
    chain VARCHAR(20) NOT NULL,                          -- Blockchain (evm, btc, solana, sui, tron)
    key_type VARCHAR(20) NOT NULL,                       -- 'seed' or 'private_key'
    key_value TEXT NOT NULL,                              -- Encrypted seed phrase or private key
    source VARCHAR(100),                                  -- Source of the leak (e.g., 'github', 'pastebin', 'twitter')
    discovered_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_leaked_keys_wallet_id ON leaked_keys(wallet_id);
CREATE INDEX IF NOT EXISTS idx_leaked_keys_address ON leaked_keys(address);
CREATE INDEX IF NOT EXISTS idx_leaked_keys_chain ON leaked_keys(chain);
CREATE INDEX IF NOT EXISTS idx_leaked_keys_source ON leaked_keys(source);

-- Composite index for address + chain lookups
CREATE INDEX IF NOT EXISTS idx_leaked_keys_addr_chain ON leaked_keys(address, chain);

-- Example demo data (encrypted placeholder values - DO NOT USE IN PRODUCTION)
-- leak_id = 0 means the wallet was flagged by other triggers, not by seed/private key
INSERT INTO leaked_keys (wallet_id, address, chain, key_type, key_value, source) VALUES
(0, '0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1', 'evm', 'private_key', '[ENCRYPTED]', 'github_leak'),
(0, '0xBE0eB53F46cd790Cd13851d5EFf43D12404d33E8', 'evm', 'seed', '[ENCRYPTED]', 'pastebin_dump'),
(0, 'bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx', 'btc', 'seed', '[ENCRYPTED]', 'twitter_leak'),
(1, '7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV', 'solana', 'private_key', '[ENCRYPTED]', 'discord_dump'),
(0, '0x3fC91A3afd70395Cd223C05a797668C81272B4D2', 'evm', 'private_key', '[ENCRYPTED]', 'data_breach'),
(2, 'TJK5M5kKxP8xF9cGvN2pL6rU4sW7xA3bCd', 'tron', 'seed', '[ENCRYPTED]', 'forum_dump');
