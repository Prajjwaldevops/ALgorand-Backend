-- ============================================================
-- BountyVault v3.1 — Database Migration
-- Run in Supabase SQL Editor
-- Architecture: Aligned with prompt.docx v3.1 corrected spec
-- ============================================================

-- ============================================================
-- STEP 1: Drop old types & recreate enums (idempotent)
-- ============================================================

DO $$ BEGIN
  CREATE TYPE user_role AS ENUM ('creator', 'freelancer', 'admin');
EXCEPTION WHEN duplicate_object THEN
  -- Alter existing enum to add 'admin' if missing
  BEGIN
    ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'admin';
  EXCEPTION WHEN others THEN NULL; END;
END $$;

DO $$ BEGIN
  CREATE TYPE bounty_status AS ENUM (
    'open',          -- funded, awaiting submissions
    'in_progress',   -- at least one submission received
    'completed',     -- payout done (approval or DAO freelancer win)
    'disputed',      -- DAO court active
    'expired',       -- deadline passed, or all rejections exhausted
    'cancelled'      -- creator cancelled (0 submissions)
  );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE submission_status AS ENUM (
    'pending',       -- awaiting creator review
    'under_review',  -- creator has opened submission
    'approved',      -- creator approved + payout done
    'rejected'       -- creator rejected
  );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE dispute_status AS ENUM (
    'open',                  -- 48hr DAO window open
    'resolved_creator',      -- creator won: tie or creator leads → ALGO refunded
    'resolved_freelancer',   -- freelancer won: votes_freelancer > votes_creator
    'tie_resolved'           -- explicit tie → creator wins by default
  );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE dao_vote_choice AS ENUM ('creator', 'freelancer');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  CREATE TYPE txn_event AS ENUM (
    'bounty_created',
    'escrow_locked',
    'work_submitted',
    'submission_approved',
    'submission_rejected',
    'dispute_raised',
    'dao_vote_cast',
    'dao_resolved',
    'bounty_refunded',
    'bounty_cancelled',
    'bounty_expired',
    'freelancer_letgo'
  );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- ============================================================
-- STEP 2: profiles table (aligned with v3.1)
-- ============================================================

CREATE TABLE IF NOT EXISTS profiles (
  id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  clerk_id                TEXT UNIQUE NOT NULL,
  username                VARCHAR(100) UNIQUE NOT NULL,
  display_name            VARCHAR(200),
  email                   VARCHAR(320) NOT NULL,
  avatar_url              TEXT,                          -- Cloudflare R2 URL
  wallet_address          VARCHAR(58),                   -- Algorand address
  role                    user_role DEFAULT 'freelancer' NOT NULL,
  bio                     TEXT,
  reputation_score        INT DEFAULT 0 NOT NULL,
  total_bounties_created  INT DEFAULT 0 NOT NULL,
  total_bounties_completed INT DEFAULT 0 NOT NULL,       -- As freelancer
  total_earned_algo       DECIMAL(18,6) DEFAULT 0 NOT NULL, -- Lifetime ALGO earned
  streak_count            INT DEFAULT 0 NOT NULL,
  avg_rating              DECIMAL(3,2) DEFAULT 0 NOT NULL,
  total_ratings           INT DEFAULT 0 NOT NULL,
  created_at              TIMESTAMPTZ DEFAULT NOW() NOT NULL,
  updated_at              TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Alter existing profiles table to add new columns if upgrading
ALTER TABLE profiles
  ADD COLUMN IF NOT EXISTS total_earned_algo DECIMAL(18,6) DEFAULT 0 NOT NULL;

-- Fix default role to freelancer
ALTER TABLE profiles
  ALTER COLUMN role SET DEFAULT 'freelancer';

-- ============================================================
-- STEP 3: bounties table (v3.1 aligned)
-- ============================================================

CREATE TABLE IF NOT EXISTS bounties (
  id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bounty_id               VARCHAR(12) UNIQUE NOT NULL,  -- CR + 5-digit e.g. CR00847
  creator_id              UUID NOT NULL REFERENCES profiles(id) ON DELETE RESTRICT,
  title                   VARCHAR(300) NOT NULL,
  description             TEXT NOT NULL,
  reward_algo             DECIMAL(18,6) NOT NULL CHECK (reward_algo > 0),
  deadline                TIMESTAMPTZ NOT NULL,
  status                  bounty_status DEFAULT 'open' NOT NULL,
  max_submissions         INT DEFAULT 5 NOT NULL CHECK (max_submissions > 0 AND max_submissions <= 50),
  submissions_remaining   INT NOT NULL,                  -- Starts at max_submissions, decremented on rejection
  tags                    TEXT[] DEFAULT '{}',
  app_id                  BIGINT,                        -- Algorand contract App ID (set after lock confirmed)
  escrow_txn_id           VARCHAR(52),                   -- Initial lock transaction ID
  payout_txn_id           VARCHAR(52),                   -- Final payout transaction ID
  created_at              TIMESTAMPTZ DEFAULT NOW() NOT NULL,
  updated_at              TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Add missing columns if upgrading
ALTER TABLE bounties
  ADD COLUMN IF NOT EXISTS bounty_id VARCHAR(12),
  ADD COLUMN IF NOT EXISTS submissions_remaining INT,
  ADD COLUMN IF NOT EXISTS payout_txn_id VARCHAR(52);

-- Remove old columns no longer in v3.1
ALTER TABLE bounties
  DROP COLUMN IF EXISTS arbitrator_address,
  DROP COLUMN IF EXISTS terms_ipfs_cid,
  DROP COLUMN IF EXISTS terms_hash,
  DROP COLUMN IF EXISTS category_id,
  DROP COLUMN IF EXISTS difficulty,
  DROP COLUMN IF EXISTS is_featured,
  DROP COLUMN IF EXISTS view_count;

-- ============================================================
-- STEP 4: submissions table (v3.1 aligned)
-- ============================================================

CREATE TABLE IF NOT EXISTS submissions (
  id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bounty_id               UUID NOT NULL REFERENCES bounties(id) ON DELETE CASCADE,
  freelancer_id           UUID NOT NULL REFERENCES profiles(id) ON DELETE RESTRICT,
  submission_number       INT NOT NULL,                  -- 1,2,3... per freelancer per bounty
  file_url                TEXT NOT NULL,                 -- Cloudflare R2 object key path
  file_type               VARCHAR(10) NOT NULL,          -- pdf/doc/docx/jpg/png
  file_size_bytes         INT NOT NULL CHECK (file_size_bytes <= 4999552), -- Max 4.95MB
  description             TEXT NOT NULL,                 -- Freelancer's work description
  status                  submission_status DEFAULT 'pending' NOT NULL,
  rejection_feedback      TEXT,                          -- Required when rejected (min 50 chars)
  creator_message         TEXT,                          -- Optional on approval
  creator_rating          INT CHECK (creator_rating >= 1 AND creator_rating <= 5), -- 1-5 stars on approval
  submission_txn_id       VARCHAR(52),                   -- On-chain submit_proof txn ID
  work_hash_sha256        VARCHAR(64),                   -- SHA256(r2_path + file_size) used on-chain
  reviewed_at             TIMESTAMPTZ,                   -- When creator first opened submission
  resolved_at             TIMESTAMPTZ,                   -- When accepted or rejected
  created_at              TIMESTAMPTZ DEFAULT NOW() NOT NULL,
  UNIQUE(bounty_id, freelancer_id, submission_number)
);

-- Remove old work_ipfs_cid column — submissions don't go to IPFS in v3.1
ALTER TABLE submissions
  DROP COLUMN IF EXISTS work_ipfs_cid,
  DROP COLUMN IF EXISTS work_hash,
  ADD COLUMN IF NOT EXISTS file_url TEXT,
  ADD COLUMN IF NOT EXISTS file_type VARCHAR(10),
  ADD COLUMN IF NOT EXISTS file_size_bytes INT,
  ADD COLUMN IF NOT EXISTS submission_number INT,
  ADD COLUMN IF NOT EXISTS creator_message TEXT,
  ADD COLUMN IF NOT EXISTS creator_rating INT,
  ADD COLUMN IF NOT EXISTS work_hash_sha256 VARCHAR(64),
  ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;

-- ============================================================
-- STEP 5: disputes table (v3.1 — DAO Court model)
-- ============================================================

CREATE TABLE IF NOT EXISTS disputes (
  id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dispute_id              VARCHAR(15) UNIQUE NOT NULL,   -- DSP + 6-digit e.g. DSP004821
  bounty_id               UUID NOT NULL REFERENCES bounties(id) ON DELETE RESTRICT,
  freelancer_id           UUID NOT NULL REFERENCES profiles(id), -- Who raised it
  creator_id              UUID NOT NULL REFERENCES profiles(id), -- Bounty creator
  freelancer_description  TEXT NOT NULL,                 -- Min 300 words (enforced in backend)
  submission_history      JSONB NOT NULL DEFAULT '[]',   -- All attempts: file_url, description, rejection_feedback, timestamps
  status                  dispute_status DEFAULT 'voting' NOT NULL,
  votes_creator           INT DEFAULT 0 NOT NULL,        -- Live tally
  votes_freelancer        INT DEFAULT 0 NOT NULL,        -- Live tally
  voting_deadline         TIMESTAMPTZ NOT NULL,           -- created_at + 48 hours
  resolved_at             TIMESTAMPTZ,
  resolution_txn_id       VARCHAR(52),                   -- Final on-chain release txn
  ipfs_dispute_cid        VARCHAR(100),                  -- CID of full dispute metadata on IPFS
  created_at              TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Remove old columns if upgrading
ALTER TABLE disputes
  DROP COLUMN IF EXISTS arbitrator_address,
  DROP COLUMN IF EXISTS auto_refund_after,
  DROP COLUMN IF EXISTS evidence_ipfs_cid,
  DROP COLUMN IF EXISTS resolution_notes,
  ADD COLUMN IF NOT EXISTS dispute_id VARCHAR(15),
  ADD COLUMN IF NOT EXISTS freelancer_description TEXT,
  ADD COLUMN IF NOT EXISTS submission_history JSONB DEFAULT '[]',
  ADD COLUMN IF NOT EXISTS votes_creator INT DEFAULT 0,
  ADD COLUMN IF NOT EXISTS votes_freelancer INT DEFAULT 0,
  ADD COLUMN IF NOT EXISTS voting_deadline TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS ipfs_dispute_cid VARCHAR(100);

-- ============================================================
-- STEP 6: dao_votes table (new in v3.1)
-- ============================================================

CREATE TABLE IF NOT EXISTS dao_votes (
  id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dispute_id              UUID NOT NULL REFERENCES disputes(id) ON DELETE CASCADE,
  voter_id                UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  vote                    dao_vote_choice NOT NULL,       -- 'creator' or 'freelancer'
  vote_txn_id             VARCHAR(52),                   -- On-chain cast_dao_vote txn ID
  ipfs_vote_cid           VARCHAR(100),                  -- CID of vote metadata JSON on IPFS
  voted_at                TIMESTAMPTZ DEFAULT NOW() NOT NULL,
  UNIQUE(dispute_id, voter_id)                           -- One vote per user per dispute at DB level
);

-- ============================================================
-- STEP 7: transaction_log table (v3.1 — aligned with IPFS metadata)
-- ============================================================

CREATE TABLE IF NOT EXISTS transaction_log (
  id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bounty_id               UUID REFERENCES bounties(id),
  actor_id                UUID REFERENCES profiles(id),  -- Who triggered the action
  event                   txn_event NOT NULL,
  txn_id                  VARCHAR(52),                   -- Algorand txn ID
  txn_note                VARCHAR(200),                  -- On-chain note: BountyVault:{event}:{app_id}
  ipfs_metadata_cid       VARCHAR(100),                  -- CID of transaction metadata JSON on IPFS
  ipfs_gateway_url        TEXT,                          -- https://gateway.pinata.cloud/ipfs/{CID}
  amount_algo             DECIMAL(18,6),                 -- ALGO amount if applicable
  metadata                JSONB,                         -- Full metadata object (DB backup)
  created_at              TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Migrate old transaction_log if columns differ
-- First add the 'event' column (required by admin view) before dropping old columns
ALTER TABLE transaction_log
  ADD COLUMN IF NOT EXISTS event txn_event,
  ADD COLUMN IF NOT EXISTS txn_note VARCHAR(200),
  ADD COLUMN IF NOT EXISTS ipfs_metadata_cid VARCHAR(100),
  ADD COLUMN IF NOT EXISTS ipfs_gateway_url TEXT,
  ADD COLUMN IF NOT EXISTS amount_algo DECIMAL(18,6),
  ADD COLUMN IF NOT EXISTS metadata JSONB;

-- Drop old columns that no longer exist in v3.1 (separate statement to avoid errors)
DO $$ BEGIN
  ALTER TABLE transaction_log DROP COLUMN IF EXISTS txn_type;
  ALTER TABLE transaction_log DROP COLUMN IF EXISTS from_address;
  ALTER TABLE transaction_log DROP COLUMN IF EXISTS to_address;
  ALTER TABLE transaction_log DROP COLUMN IF EXISTS status;
EXCEPTION WHEN others THEN NULL; END $$;

-- ============================================================
-- STEP 8: admin_users table (new — for admin panel auth)
-- ============================================================

CREATE TABLE IF NOT EXISTS admin_users (
  id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  username                VARCHAR(100) UNIQUE NOT NULL,
  password_hash           TEXT NOT NULL,
  display_name            VARCHAR(200),
  last_login_at           TIMESTAMPTZ,
  created_at              TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Seed default admin (password: admin — bcrypt hash)
INSERT INTO admin_users (username, password_hash, display_name)
VALUES (
  'admin',
  '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', -- bcrypt of "admin"
  'System Administrator'
)
ON CONFLICT (username) DO NOTHING;

-- ============================================================
-- STEP 9: audit_log table (admin action tracking)
-- ============================================================

CREATE TABLE IF NOT EXISTS audit_log (
  id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  admin_id                UUID REFERENCES admin_users(id),
  admin_username          VARCHAR(100) NOT NULL,
  action                  VARCHAR(100) NOT NULL,
  target_type             VARCHAR(50),                   -- 'user', 'bounty', 'dispute', 'setting'
  target_id               TEXT,
  old_value               JSONB,
  new_value               JSONB,
  ip_address              VARCHAR(45),
  created_at              TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- ============================================================
-- STEP 10: Indexes for performance
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_profiles_clerk_id ON profiles(clerk_id);
CREATE INDEX IF NOT EXISTS idx_profiles_wallet ON profiles(wallet_address);
CREATE INDEX IF NOT EXISTS idx_bounties_creator ON bounties(creator_id);
CREATE INDEX IF NOT EXISTS idx_bounties_status ON bounties(status);
CREATE INDEX IF NOT EXISTS idx_bounties_bounty_id ON bounties(bounty_id);
CREATE INDEX IF NOT EXISTS idx_submissions_bounty ON submissions(bounty_id);
CREATE INDEX IF NOT EXISTS idx_submissions_freelancer ON submissions(freelancer_id);
CREATE INDEX IF NOT EXISTS idx_submissions_status ON submissions(status);
CREATE INDEX IF NOT EXISTS idx_disputes_bounty ON disputes(bounty_id);
CREATE INDEX IF NOT EXISTS idx_disputes_status ON disputes(status);
CREATE INDEX IF NOT EXISTS idx_disputes_deadline ON disputes(voting_deadline);
CREATE INDEX IF NOT EXISTS idx_dao_votes_dispute ON dao_votes(dispute_id);
CREATE INDEX IF NOT EXISTS idx_dao_votes_voter ON dao_votes(voter_id);
CREATE INDEX IF NOT EXISTS idx_txn_log_bounty ON transaction_log(bounty_id);
CREATE INDEX IF NOT EXISTS idx_txn_log_actor ON transaction_log(actor_id);
CREATE INDEX IF NOT EXISTS idx_txn_log_event ON transaction_log(event);
CREATE INDEX IF NOT EXISTS idx_txn_log_created ON transaction_log(created_at DESC);

-- ============================================================
-- STEP 11: Triggers — auto-update updated_at
-- ============================================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS update_profiles_updated_at ON profiles;
CREATE TRIGGER update_profiles_updated_at
  BEFORE UPDATE ON profiles
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_bounties_updated_at ON bounties;
CREATE TRIGGER update_bounties_updated_at
  BEFORE UPDATE ON bounties
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- STEP 12: Helper function — generate bounty_id (CR + random 5-digit)
-- ============================================================

CREATE OR REPLACE FUNCTION generate_bounty_id()
RETURNS VARCHAR(12) AS $$
DECLARE
  new_id VARCHAR(12);
  counter INT := 0;
BEGIN
  LOOP
    new_id := 'CR' || LPAD(FLOOR(RANDOM() * 99999 + 1)::TEXT, 5, '0');
    IF NOT EXISTS (SELECT 1 FROM bounties WHERE bounty_id = new_id) THEN
      RETURN new_id;
    END IF;
    counter := counter + 1;
    IF counter > 100 THEN
      RAISE EXCEPTION 'Could not generate unique bounty_id after 100 attempts';
    END IF;
  END LOOP;
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- STEP 13: Helper function — generate dispute_id (DSP + random 6-digit)
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
-- STEP 14: Views for admin dashboard
-- ============================================================

CREATE OR REPLACE VIEW admin_platform_stats AS
SELECT
  (SELECT COUNT(*) FROM profiles WHERE role = 'freelancer')    AS total_freelancers,
  (SELECT COUNT(*) FROM profiles WHERE role = 'creator')       AS total_creators,
  (SELECT COUNT(*) FROM bounties)                              AS total_bounties,
  (SELECT COUNT(*) FROM bounties WHERE status = 'open')        AS open_bounties,
  (SELECT COUNT(*) FROM bounties WHERE status = 'in_progress') AS in_progress_bounties,
  (SELECT COUNT(*) FROM bounties WHERE status = 'completed')   AS completed_bounties,
  (SELECT COUNT(*) FROM bounties WHERE status = 'disputed')    AS disputed_bounties,
  (SELECT COUNT(*) FROM submissions)                           AS total_submissions,
  (SELECT COUNT(*) FROM submissions WHERE status = 'approved') AS accepted_submissions,
  (SELECT COALESCE(SUM(reward_algo), 0) FROM bounties)         AS total_algo_volume,
  (SELECT COALESCE(SUM(amount_algo), 0) FROM transaction_log WHERE event = 'submission_approved') AS total_algo_paid_out,
  (SELECT COUNT(*) FROM disputes WHERE status = 'open')      AS active_disputes,
  (SELECT COUNT(*) FROM transaction_log)                       AS total_transactions;

-- ============================================================
-- STEP 15: RLS Policies
-- ============================================================

-- Enable RLS on all new tables
ALTER TABLE dao_votes ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE admin_users ENABLE ROW LEVEL SECURITY;

-- Profiles: users can read all, write own
DROP POLICY IF EXISTS "profiles_select" ON profiles;
CREATE POLICY "profiles_select" ON profiles FOR SELECT USING (true);
DROP POLICY IF EXISTS "profiles_update_own" ON profiles;
CREATE POLICY "profiles_update_own" ON profiles FOR UPDATE USING (auth.uid()::text = clerk_id);

-- Bounties: public read
DROP POLICY IF EXISTS "bounties_select" ON bounties;
CREATE POLICY "bounties_select" ON bounties FOR SELECT USING (true);

-- dao_votes: public read, authenticated write
DROP POLICY IF EXISTS "dao_votes_select" ON dao_votes;
CREATE POLICY "dao_votes_select" ON dao_votes FOR SELECT USING (true);

-- transaction_log: public read (for IPFS audit trail)
DROP POLICY IF EXISTS "txn_log_select" ON transaction_log;
CREATE POLICY "txn_log_select" ON transaction_log FOR SELECT USING (true);

-- admin_users: no direct client access
DROP POLICY IF EXISTS "admin_users_deny_all" ON admin_users;
CREATE POLICY "admin_users_deny_all" ON admin_users FOR ALL USING (false);
