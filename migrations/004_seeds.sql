-- Migration: 004_seeds.sql
-- Table for storing seed phrases with wallet references

-- Seeds table
CREATE TABLE IF NOT EXISTS seeds (
    id BIGSERIAL PRIMARY KEY,
    seed_phrase TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for seeds
CREATE INDEX IF NOT EXISTS idx_seeds_id ON seeds(id);

-- Add seed_id column to wallets table (0 = no seed)
ALTER TABLE wallets ADD COLUMN IF NOT EXISTS seed_id BIGINT DEFAULT 0;

-- Add reason column to wallets table (e.g., 'leaked seed', 'pk leak', 'phishing')
ALTER TABLE wallets ADD COLUMN IF NOT EXISTS reason VARCHAR(100);

-- Add source column to wallets table (e.g., 'github', 'discord', 'manual')
ALTER TABLE wallets ADD COLUMN IF NOT EXISTS source VARCHAR(100);
