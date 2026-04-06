-- ============================================================
-- BountyVault v3.3 — Full Bounty Lifecycle Migration
-- Run in Supabase SQL Editor AFTER migration-v3.2-acceptances.sql
-- ============================================================

-- ============================================================
-- STEP 1: Add 'accepted' to bounty_status enum
-- ============================================================
DO $$ BEGIN
  ALTER TYPE bounty_status ADD VALUE IF NOT EXISTS 'accepted' AFTER 'open';
EXCEPTION WHEN others THEN NULL; END $$;

-- ============================================================
-- STEP 2: Add accepted_freelancer_id to bounties
-- ============================================================
ALTER TABLE bounties
  ADD COLUMN IF NOT EXISTS accepted_freelancer_id UUID REFERENCES profiles(id);

CREATE INDEX IF NOT EXISTS idx_bounties_accepted_freelancer ON bounties(accepted_freelancer_id);

-- ============================================================
-- STEP 3: Add mega.nz and encryption key fields to submissions
-- ============================================================
ALTER TABLE submissions
  ADD COLUMN IF NOT EXISTS mega_nz_link TEXT,
  ADD COLUMN IF NOT EXISTS encryption_key_r2_path TEXT,
  ADD COLUMN IF NOT EXISTS encryption_key_r2_url TEXT;

-- Make file_url nullable (no longer required — work files go to mega.nz)
ALTER TABLE submissions ALTER COLUMN file_url DROP NOT NULL;

-- ============================================================
-- STEP 4: Add new transaction events
-- ============================================================
DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'bounty_accepted';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'work_resubmitted';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'dispute_freelancer_wins';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'dispute_creator_wins';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'dispute_tie_creator_wins';
EXCEPTION WHEN others THEN NULL; END $$;

-- ============================================================
-- STEP 5: Fix dispute_status enum — ensure 'open' exists
-- ============================================================
DO $$ BEGIN
  ALTER TYPE dispute_status ADD VALUE IF NOT EXISTS 'open';
EXCEPTION WHEN others THEN NULL; END $$;

-- ============================================================
-- STEP 6: Relax unique constraint on submissions to allow resubmissions
-- Drop and recreate with just bounty_id + freelancer_id + submission_number
-- ============================================================
-- The existing UNIQUE(bounty_id, freelancer_id, submission_number) already
-- supports resubmissions as long as submission_number increments.

-- ============================================================
-- STEP 7: Add transaction log index for efficient user lookups
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_txn_log_ipfs ON transaction_log(ipfs_metadata_cid)
  WHERE ipfs_metadata_cid IS NOT NULL;

-- ============================================================
-- STEP 8: Update admin view to include accepted bounties
-- ============================================================
DROP VIEW IF EXISTS admin_platform_stats;
CREATE VIEW admin_platform_stats AS
SELECT
  (SELECT COUNT(*) FROM profiles WHERE role = 'freelancer')                AS total_freelancers,
  (SELECT COUNT(*) FROM profiles WHERE role = 'creator')                   AS total_creators,
  (SELECT COUNT(*) FROM bounties)                                          AS total_bounties,
  (SELECT COUNT(*) FROM bounties WHERE status::text = 'open')              AS open_bounties,
  (SELECT COUNT(*) FROM bounties WHERE status::text = 'accepted')          AS accepted_bounties,
  (SELECT COUNT(*) FROM bounties WHERE status::text = 'in_progress')       AS in_progress_bounties,
  (SELECT COUNT(*) FROM bounties WHERE status::text = 'completed')         AS completed_bounties,
  (SELECT COUNT(*) FROM bounties WHERE status::text = 'disputed')          AS disputed_bounties,
  (SELECT COUNT(*) FROM submissions)                                       AS total_submissions,
  (SELECT COUNT(*) FROM submissions WHERE status = 'approved')             AS accepted_submissions,
  (SELECT COALESCE(SUM(reward_algo), 0) FROM bounties)                     AS total_algo_volume,
  (SELECT COALESCE(SUM(amount_algo), 0) FROM transaction_log WHERE event::text = 'submission_approved') AS total_algo_paid_out,
  (SELECT COUNT(*) FROM disputes WHERE status::text = 'open')              AS active_disputes,
  (SELECT COUNT(*) FROM transaction_log)                                   AS total_transactions;

-- ============================================================
-- DONE
-- ============================================================
