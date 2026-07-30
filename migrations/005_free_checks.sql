-- Migration: 005_free_checks.sql
-- Add columns for tracking daily free checks per user

-- Add free check tracking columns to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS free_checks_used INTEGER DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS free_checks_reset_at TIMESTAMP WITH TIME ZONE DEFAULT NOW();
