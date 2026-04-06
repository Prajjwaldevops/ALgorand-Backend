-- ============================================================
-- BountyVault v3.4 — Payout & Wallet Tracking Migration
-- Run in Supabase SQL Editor AFTER migration-v3.3-lifecycle.sql
-- ============================================================

-- STEP 1: Store freelancer wallet address at submission time
ALTER TABLE submissions ADD COLUMN IF NOT EXISTS freelancer_wallet_address TEXT;

-- STEP 2: Track payout transaction hash after creator approval
ALTER TABLE submissions ADD COLUMN IF NOT EXISTS payout_txn_hash TEXT;

-- ============================================================
-- DONE
-- ============================================================
