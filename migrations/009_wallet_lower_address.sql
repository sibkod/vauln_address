-- Migration: 009_wallet_lower_address.sql
-- Speeds up the EVM bulk/existence lookups on MySQL: the LOWER(address)
-- predicate on varchar indexes scans the whole wallets table (≈10 s at
-- ~300k rows), which deadlocks the live-block scanners. A STORED generated
-- column plus a (chain, lower_address) index turns the same predicate into
-- an index lookup. SQLite/PostgreSQL keep using LOWER(address).

-- MySQL
ALTER TABLE wallets ADD COLUMN lower_address VARCHAR(100)
    GENERATED ALWAYS AS (LOWER(address)) STORED;
CREATE INDEX idx_wallets_chain_lower ON wallets(chain, lower_address);
