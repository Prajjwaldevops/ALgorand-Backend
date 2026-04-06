-- ============================================================
-- BountyVault v3.2 — Bounty Acceptance Flow Migration
-- Run in Supabase SQL Editor AFTER migration-v3.1.sql
-- ============================================================

-- ============================================================
-- STEP 1: bounty_acceptances table
-- Tracks freelancer requests to accept a bounty.
-- Flow: freelancer requests → creator reviews → approve triggers escrow
-- ============================================================

CREATE TABLE IF NOT EXISTS bounty_acceptances (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bounty_id       UUID NOT NULL REFERENCES bounties(id) ON DELETE CASCADE,
  freelancer_id   UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  status          VARCHAR(20) DEFAULT 'pending' NOT NULL,  -- pending, approved, rejected
  message         TEXT,                                     -- optional message from freelancer
  creator_note    TEXT,                                     -- optional note from creator
  created_at      TIMESTAMPTZ DEFAULT NOW() NOT NULL,
  updated_at      TIMESTAMPTZ DEFAULT NOW() NOT NULL,
  UNIQUE(bounty_id, freelancer_id)                          -- one request per freelancer per bounty
);

-- ============================================================
-- STEP 2: Indexes
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_acceptances_bounty ON bounty_acceptances(bounty_id);
CREATE INDEX IF NOT EXISTS idx_acceptances_freelancer ON bounty_acceptances(freelancer_id);
CREATE INDEX IF NOT EXISTS idx_acceptances_status ON bounty_acceptances(status);

-- ============================================================
-- STEP 3: Trigger for updated_at
-- ============================================================

DROP TRIGGER IF EXISTS update_acceptances_updated_at ON bounty_acceptances;
CREATE TRIGGER update_acceptances_updated_at
  BEFORE UPDATE ON bounty_acceptances
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- STEP 4: Add bounty_acceptance_created to txn_event enum
-- ============================================================

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'bounty_acceptance_created';
EXCEPTION WHEN others THEN NULL; END $$;

DO $$ BEGIN
  ALTER TYPE txn_event ADD VALUE IF NOT EXISTS 'bounty_acceptance_approved';
EXCEPTION WHEN others THEN NULL; END $$;

-- ============================================================
-- STEP 5: Notifications table (create if not exists)
-- Required for acceptance notifications
-- ============================================================

CREATE TABLE IF NOT EXISTS notifications (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  type            VARCHAR(50) NOT NULL,
  title           VARCHAR(200) NOT NULL,
  message         TEXT NOT NULL,
  bounty_id       UUID REFERENCES bounties(id) ON DELETE SET NULL,
  is_read         BOOLEAN DEFAULT FALSE NOT NULL,
  created_at      TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_read ON notifications(user_id, is_read);
