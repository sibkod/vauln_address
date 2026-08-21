-- Migration: 005_scan_findings.sql
-- Drainer scanner findings (solana_scan.py) and user drainer reports.
-- The runtime schema (repository.InitSchema) creates the same tables for
-- sqlite/mysql/postgres; this file documents the postgres variant.

-- Findings detected by solana_scan.py: one row per flagged transaction.
-- victim_address = drained/hijacked wallet, hacker_address = sweep
-- destination or takeover program.
CREATE TABLE IF NOT EXISTS scan_findings (
    id BIGSERIAL PRIMARY KEY,
    chain VARCHAR(20) NOT NULL DEFAULT 'solana',
    signature VARCHAR(120) NOT NULL UNIQUE,
    slot BIGINT DEFAULT 0,
    verdict VARCHAR(20) NOT NULL,
    indicators TEXT,
    victim_address VARCHAR(100),
    hacker_address VARCHAR(100),
    amount_sol DECIMAL(20, 9) DEFAULT 0,
    programs TEXT,
    source VARCHAR(20),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scan_findings_created ON scan_findings(created_at);
CREATE INDEX IF NOT EXISTS idx_scan_findings_victim ON scan_findings(victim_address);
CREATE INDEX IF NOT EXISTS idx_scan_findings_hacker ON scan_findings(hacker_address);

-- User-submitted drainer reports (captcha-protected, forwarded to Telegram).
CREATE TABLE IF NOT EXISTS drainer_reports (
    id BIGSERIAL PRIMARY KEY,
    tx_signature VARCHAR(120) NOT NULL,
    chain VARCHAR(20) NOT NULL DEFAULT 'solana',
    site_url VARCHAR(300),
    description TEXT,
    reporter VARCHAR(100) DEFAULT '',
    status VARCHAR(20) DEFAULT 'new',
    telegram_sent BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_drainer_reports_created ON drainer_reports(created_at);
