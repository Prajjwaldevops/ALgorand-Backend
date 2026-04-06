-- ============================================================
-- BountyVault v3.1 Fix — Add missing tables
-- Run in Supabase SQL Editor if dashboard/notifications fail
-- ============================================================

-- 1. Create notifications table if missing
CREATE TABLE IF NOT EXISTS notifications (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  type          TEXT NOT NULL,
  title         VARCHAR(200) NOT NULL,
  message       TEXT NOT NULL,
  bounty_id     UUID REFERENCES bounties(id) ON DELETE SET NULL,
  is_read       BOOLEAN DEFAULT FALSE,
  metadata      JSONB,
  created_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_unread ON notifications(user_id, is_read) WHERE is_read = FALSE;

-- 2. Create bounty_status_updates table if missing
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

-- 3. Add first_name and last_name columns if missing
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS first_name VARCHAR(100);
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS last_name VARCHAR(100);

-- 4. Fix column name: ensure total_earned_algo exists (some schemas use total_earnings_algo)
DO $$
BEGIN
  -- If total_earnings_algo exists but total_earned_algo does not, rename it
  IF EXISTS (
    SELECT 1 FROM information_schema.columns 
    WHERE table_name = 'profiles' AND column_name = 'total_earnings_algo'
  ) AND NOT EXISTS (
    SELECT 1 FROM information_schema.columns 
    WHERE table_name = 'profiles' AND column_name = 'total_earned_algo'
  ) THEN
    ALTER TABLE profiles RENAME COLUMN total_earnings_algo TO total_earned_algo;
  END IF;
  
  -- If neither exists, add total_earned_algo
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns 
    WHERE table_name = 'profiles' AND column_name = 'total_earned_algo'
  ) THEN
    ALTER TABLE profiles ADD COLUMN total_earned_algo DECIMAL(18,6) DEFAULT 0 NOT NULL;
  END IF;
END $$;

-- ============================================================
-- END OF FIX MIGRATION
-- ============================================================
