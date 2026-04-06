-- ============================================================
-- Migration: Add first_name, last_name to profiles
--            Add bounty_status_updates table
-- BountyVault — Role-Based Dashboards Enhancement
-- ============================================================

-- 1. Add first_name and last_name to profiles
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS first_name VARCHAR(100);
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS last_name VARCHAR(100);

-- 2. Bounty Status Updates table — tracks progress updates from freelancers/creators
CREATE TABLE IF NOT EXISTS bounty_status_updates (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bounty_id     UUID NOT NULL REFERENCES bounties(id) ON DELETE CASCADE,
  updated_by    UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  old_status    VARCHAR(30),
  new_status    VARCHAR(30) NOT NULL,
  note          TEXT,
  created_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_status_updates_bounty ON bounty_status_updates(bounty_id);
CREATE INDEX IF NOT EXISTS idx_status_updates_user ON bounty_status_updates(updated_by);
CREATE INDEX IF NOT EXISTS idx_status_updates_created ON bounty_status_updates(created_at DESC);

-- RLS for status updates
ALTER TABLE bounty_status_updates ENABLE ROW LEVEL SECURITY;
CREATE POLICY status_updates_select ON bounty_status_updates FOR SELECT USING (true);
CREATE POLICY status_updates_insert ON bounty_status_updates FOR INSERT WITH CHECK (
  updated_by IN (SELECT id FROM profiles WHERE clerk_id = current_setting('app.current_clerk_id', true))
);

-- ============================================================
-- END OF MIGRATION
-- ============================================================
