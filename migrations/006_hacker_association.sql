-- Migration: 006_hacker_association.sql
-- Flags wallets that transferred funds to a known hacker/drainer operator.

-- PostgreSQL
ALTER TABLE wallets ADD COLUMN IF NOT EXISTS associated_hacker BOOLEAN DEFAULT FALSE;
ALTER TABLE wallets ADD COLUMN IF NOT EXISTS associated_reason VARCHAR(255);

CREATE INDEX IF NOT EXISTS idx_wallets_associated_hacker ON wallets(associated_hacker);
