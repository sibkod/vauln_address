-- Migration: 007_scan_exposed.sql
-- Persists the scanner's funding sources per finding (flow-trace payers),
-- so reports can link payouts back to the operator/program that made them.

-- PostgreSQL
ALTER TABLE scan_findings ADD COLUMN IF NOT EXISTS exposed_addresses TEXT;
