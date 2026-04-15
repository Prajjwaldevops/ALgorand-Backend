-- ============================================================
-- BountyVault v3.6 — Mandatory DAO Voting Compliance
-- Adds tracking for monthly vote requirement & ban enforcement
-- ============================================================

-- Add voting compliance columns to profiles
ALTER TABLE profiles
  ADD COLUMN IF NOT EXISTS last_dao_vote_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS is_dao_banned BOOLEAN DEFAULT FALSE;

-- Add dao_vote_cast to txn_event enum if not present
DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'dao_vote_cast';
EXCEPTION WHEN others THEN NULL; END $$;

-- ============================================================
-- END OF MIGRATION v3.6
-- ============================================================
