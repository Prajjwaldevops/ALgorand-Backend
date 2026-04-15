-- ============================================================
-- BountyVault v3.5 — DAO Dispute Resolution Migration
-- Adds missing transaction event types for dispute flows
-- AND fixes the disputes table schema to match the v3.1 code
-- ============================================================

-- ============================================================
-- STEP 1: Fix dispute_status enum
-- ============================================================

-- Add missing enum values to dispute_status
DO $$ BEGIN
  ALTER TYPE dispute_status ADD VALUE IF NOT EXISTS 'open';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE dispute_status ADD VALUE IF NOT EXISTS 'resolved_creator';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE dispute_status ADD VALUE IF NOT EXISTS 'resolved_freelancer';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE dispute_status ADD VALUE IF NOT EXISTS 'tie_resolved';
EXCEPTION WHEN others THEN NULL; END $$;

-- Create dao_vote_choice enum if not exists
DO $$ BEGIN
  CREATE TYPE dao_vote_choice AS ENUM ('creator', 'freelancer');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- ============================================================
-- STEP 2: Fix txn_event enum — add missing event types
-- ============================================================

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'work_resubmitted';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'freelancer_letgo_refund';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'dispute_refund';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'dao_finalize_payout';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'dispute_raised';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'dao_resolved';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'freelancer_letgo';
EXCEPTION WHEN others THEN NULL; END $$;

-- ============================================================
-- STEP 3: Update disputes table — add missing columns
-- The original schema has: id, bounty_id, submission_id, initiated_by,
--   reason, evidence_ipfs_cid, status, arbitrator_address, resolution_notes,
--   resolved_at, auto_refund_after, dao_vote_deadline, created_at
-- The code expects: dispute_id, freelancer_id, creator_id,
--   freelancer_description, submission_history, votes_creator, 
--   votes_freelancer, voting_deadline, resolution_txn_id, ipfs_dispute_cid
-- ============================================================

ALTER TABLE disputes
  ADD COLUMN IF NOT EXISTS dispute_id VARCHAR(15) UNIQUE,
  ADD COLUMN IF NOT EXISTS freelancer_id UUID REFERENCES profiles(id),
  ADD COLUMN IF NOT EXISTS creator_id UUID REFERENCES profiles(id),
  ADD COLUMN IF NOT EXISTS freelancer_description TEXT,
  ADD COLUMN IF NOT EXISTS submission_history JSONB DEFAULT '[]',
  ADD COLUMN IF NOT EXISTS votes_creator INT DEFAULT 0,
  ADD COLUMN IF NOT EXISTS votes_freelancer INT DEFAULT 0,
  ADD COLUMN IF NOT EXISTS voting_deadline TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS resolution_txn_id VARCHAR(52),
  ADD COLUMN IF NOT EXISTS ipfs_dispute_cid VARCHAR(100);

-- Backfill freelancer_id from initiated_by for existing rows
UPDATE disputes SET freelancer_id = initiated_by WHERE freelancer_id IS NULL AND initiated_by IS NOT NULL;

-- Set voting_deadline from dao_vote_deadline for existing rows
UPDATE disputes SET voting_deadline = dao_vote_deadline WHERE voting_deadline IS NULL AND dao_vote_deadline IS NOT NULL;

-- Backfill freelancer_description from reason for existing rows
UPDATE disputes SET freelancer_description = reason WHERE freelancer_description IS NULL AND reason IS NOT NULL;

-- ============================================================
-- STEP 4: Fix dao_votes table — ensure correct schema exists
-- ============================================================

-- Create dao_votes table if not exists (may already exist from old schema)
CREATE TABLE IF NOT EXISTS dao_votes (
  id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dispute_id              UUID NOT NULL REFERENCES disputes(id) ON DELETE CASCADE,
  voter_id                UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  vote                    dao_vote_choice NOT NULL,
  vote_txn_id             VARCHAR(52),
  ipfs_vote_cid           VARCHAR(100),
  voted_at                TIMESTAMPTZ DEFAULT NOW() NOT NULL,
  UNIQUE(dispute_id, voter_id)
);

-- If old dao_votes exists with different column names, add missing columns
ALTER TABLE dao_votes
  ADD COLUMN IF NOT EXISTS vote_txn_id VARCHAR(52),
  ADD COLUMN IF NOT EXISTS ipfs_vote_cid VARCHAR(100),
  ADD COLUMN IF NOT EXISTS voted_at TIMESTAMPTZ DEFAULT NOW();

-- ============================================================
-- STEP 5: Create generate_dispute_id function
-- ============================================================

CREATE OR REPLACE FUNCTION generate_dispute_id()
RETURNS VARCHAR(15) AS $$
DECLARE
  new_id VARCHAR(15);
  counter INT := 0;
BEGIN
  LOOP
    new_id := 'DSP' || LPAD(FLOOR(RANDOM() * 999999 + 1)::TEXT, 6, '0');
    IF NOT EXISTS (SELECT 1 FROM disputes WHERE dispute_id = new_id) THEN
      RETURN new_id;
    END IF;
    counter := counter + 1;
    IF counter > 100 THEN
      RAISE EXCEPTION 'Could not generate unique dispute_id after 100 attempts';
    END IF;
  END LOOP;
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- STEP 6: Ensure transaction_log has needed columns
-- ============================================================

ALTER TABLE transaction_log
  ADD COLUMN IF NOT EXISTS actor_id UUID,
  ADD COLUMN IF NOT EXISTS amount_algo DECIMAL(20,6);

-- ============================================================
-- END OF MIGRATION
-- ============================================================
