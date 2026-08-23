-- Migration: 008_scan_sweeps.sql
-- Persists the per-recipient breakdown of a split drain: drainers divide the
-- stolen funds across several destination wallets in one transaction, and the
-- primary hacker_address alone cannot represent that. Format: CSV of
-- "address:amount_sol" pairs (base58 never contains ':' or ',').

-- PostgreSQL
ALTER TABLE scan_findings ADD COLUMN IF NOT EXISTS sweeps TEXT;
