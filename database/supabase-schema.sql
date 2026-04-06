-- ============================================================
-- BountyVault — Supabase PostgreSQL Schema (PRD v2.0 Enhanced)
-- AlgoBharat Hackathon | April 2026
--
-- Enhanced with features from AlgoBounty reference:
--   - DAO voting records
--   - Worker ratings & reputation
--   - Notifications
--   - Leaderboard
--   - Bounty categories
-- ============================================================

-- ========================
-- 1. ENUMS
-- ========================

CREATE TYPE user_role AS ENUM ('creator', 'worker', 'admin', 'arbitrator');

CREATE TYPE bounty_status AS ENUM (
  'open',
  'in_progress',
  'submitted',
  'completed',
  'disputed',
  'arbitrating',
  'expired',
  'cancelled',
  'paused'
);

CREATE TYPE submission_status AS ENUM (
  'pending',
  'approved',
  'rejected',
  'disputed'
);

CREATE TYPE dispute_status AS ENUM (
  'open',
  'resolved_creator',
  'resolved_worker',
  'escalated',
  'auto_refunded',
  'dao_resolved'
);

CREATE TYPE vote_choice AS ENUM ('approve', 'reject');

CREATE TYPE notification_type AS ENUM (
  'bounty_created',
  'submission_received',
  'submission_approved',
  'submission_rejected',
  'dispute_initiated',
  'dispute_resolved',
  'dao_vote_started',
  'dao_vote_ended',
  'deadline_warning',
  'payout_received',
  'streak_bonus',
  'rating_received'
);

-- ========================
-- 2. USER TABLE (Better Auth)
-- ========================

CREATE TABLE IF NOT EXISTS "user" (
  id            TEXT PRIMARY KEY,
  email         TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  email_verified BOOLEAN DEFAULT FALSE,
  created_at    TIMESTAMPTZ DEFAULT NOW(),
  updated_at    TIMESTAMPTZ DEFAULT NOW()
);

-- ========================
-- 3. PROFILES TABLE
-- ========================

CREATE TABLE IF NOT EXISTS profiles (
  id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  auth_user_id              TEXT UNIQUE NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
  username                  VARCHAR(100) UNIQUE NOT NULL,
  display_name              VARCHAR(200),
  avatar_url                TEXT,           -- Cloudflare R2 URL
  wallet_address            VARCHAR(58),    -- Algorand address (58 chars, base32)
  role                      user_role DEFAULT 'worker',
  bio                       TEXT,
  reputation_score          INT DEFAULT 0,
  total_bounties_created    INT DEFAULT 0,
  total_bounties_completed  INT DEFAULT 0,
  total_earnings_algo       DECIMAL(18,6) DEFAULT 0,
  streak_count              INT DEFAULT 0,  -- [NEW] Current completion streak
  avg_rating                DECIMAL(3,2) DEFAULT 0, -- [NEW] Average rating (1.00-5.00)
  total_ratings             INT DEFAULT 0,  -- [NEW] Number of ratings received
  created_at                TIMESTAMPTZ DEFAULT NOW(),
  updated_at                TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_profiles_wallet ON profiles(wallet_address);
CREATE INDEX idx_profiles_username ON profiles(username);
CREATE INDEX idx_profiles_role ON profiles(role);
CREATE INDEX idx_profiles_reputation ON profiles(reputation_score DESC);

-- ========================
-- 4. BOUNTY CATEGORIES TABLE [NEW]
-- ========================

CREATE TABLE IF NOT EXISTS bounty_categories (
  id          SERIAL PRIMARY KEY,
  name        VARCHAR(50) UNIQUE NOT NULL,
  slug        VARCHAR(50) UNIQUE NOT NULL,
  description TEXT,
  icon        VARCHAR(50),        -- Lucide icon name
  color       VARCHAR(7),         -- Hex color code
  created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Seed default categories
INSERT INTO bounty_categories (name, slug, description, icon, color) VALUES
  ('Development', 'development', 'Software development tasks', 'Code', '#3B82F6'),
  ('Design', 'design', 'UI/UX and graphic design', 'Palette', '#8B5CF6'),
  ('Writing', 'writing', 'Content writing and documentation', 'PenTool', '#10B981'),
  ('Research', 'research', 'Research and analysis tasks', 'Search', '#F59E0B'),
  ('Smart Contracts', 'smart-contracts', 'Blockchain smart contract work', 'FileCode', '#EF4444'),
  ('Testing', 'testing', 'QA and testing tasks', 'Bug', '#06B6D4'),
  ('Community', 'community', 'Community management tasks', 'Users', '#EC4899'),
  ('Other', 'other', 'Miscellaneous tasks', 'MoreHorizontal', '#6B7280')
ON CONFLICT (slug) DO NOTHING;

-- ========================
-- 5. BOUNTIES TABLE
-- ========================

CREATE TABLE IF NOT EXISTS bounties (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  creator_id          UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  title               VARCHAR(300) NOT NULL,
  description         TEXT NOT NULL,
  reward_algo         DECIMAL(18,6) NOT NULL CHECK (reward_algo > 0),
  terms_ipfs_cid      VARCHAR(100),           -- Pinata CID
  terms_hash          BYTEA,                  -- SHA-256 (32 bytes)
  deadline            TIMESTAMPTZ NOT NULL,
  status              bounty_status DEFAULT 'open',
  app_id              BIGINT,                 -- Algorand application ID
  escrow_txn_id       VARCHAR(52),            -- Algorand transaction ID
  arbitrator_address  VARCHAR(58),            -- Algorand address of arbitrator [v2]
  max_submissions     INT DEFAULT 5 CHECK (max_submissions > 0 AND max_submissions <= 50),
  tags                TEXT[],                 -- Array of tag strings
  category_id         INT REFERENCES bounty_categories(id), -- [NEW] Category
  difficulty          VARCHAR(20) DEFAULT 'medium' CHECK (difficulty IN ('easy', 'medium', 'hard', 'expert')),
  is_featured         BOOLEAN DEFAULT FALSE,  -- [NEW] Featured bounties
  view_count          INT DEFAULT 0,          -- [NEW] View tracking
  created_at          TIMESTAMPTZ DEFAULT NOW(),
  updated_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_bounties_creator ON bounties(creator_id);
CREATE INDEX idx_bounties_status ON bounties(status);
CREATE INDEX idx_bounties_deadline ON bounties(deadline);
CREATE INDEX idx_bounties_reward ON bounties(reward_algo);
CREATE INDEX idx_bounties_tags ON bounties USING GIN(tags);
CREATE INDEX idx_bounties_category ON bounties(category_id);
CREATE INDEX idx_bounties_difficulty ON bounties(difficulty);
CREATE INDEX idx_bounties_featured ON bounties(is_featured) WHERE is_featured = TRUE;

-- ========================
-- 6. SUBMISSIONS TABLE
-- ========================

CREATE TABLE IF NOT EXISTS submissions (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bounty_id           UUID NOT NULL REFERENCES bounties(id) ON DELETE CASCADE,
  worker_id           UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  work_ipfs_cid       VARCHAR(100) NOT NULL,
  work_hash           BYTEA NOT NULL,         -- SHA-256 (32 bytes)
  description         TEXT,
  submission_txn_id   VARCHAR(52),            -- Algorand transaction ID
  status              submission_status DEFAULT 'pending',
  feedback            TEXT,                   -- Creator feedback on rejection
  submitted_at        TIMESTAMPTZ DEFAULT NOW(),
  reviewed_at         TIMESTAMPTZ,

  -- One submission per worker per bounty
  UNIQUE(bounty_id, worker_id)
);

CREATE INDEX idx_submissions_bounty ON submissions(bounty_id);
CREATE INDEX idx_submissions_worker ON submissions(worker_id);
CREATE INDEX idx_submissions_status ON submissions(status);

-- ========================
-- 7. DISPUTES TABLE
-- ========================

CREATE TABLE IF NOT EXISTS disputes (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bounty_id           UUID NOT NULL REFERENCES bounties(id) ON DELETE CASCADE,
  submission_id       UUID REFERENCES submissions(id) ON DELETE SET NULL,
  initiated_by        UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  reason              TEXT NOT NULL,
  evidence_ipfs_cid   VARCHAR(100),
  status              dispute_status DEFAULT 'open',
  arbitrator_address  VARCHAR(58),            -- Algorand arbitrator address [v2]
  resolution_notes    TEXT,
  resolved_at         TIMESTAMPTZ,
  auto_refund_after   TIMESTAMPTZ,           -- Arbitrator deadline — 30 days [v2]
  dao_vote_deadline   TIMESTAMPTZ,           -- [NEW] DAO voting deadline (3 days)
  created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_disputes_bounty ON disputes(bounty_id);
CREATE INDEX idx_disputes_status ON disputes(status);

-- ========================
-- 8. DAO VOTES TABLE [NEW] — From AlgoBounty reference
-- ========================

CREATE TABLE IF NOT EXISTS dao_votes (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  dispute_id    UUID NOT NULL REFERENCES disputes(id) ON DELETE CASCADE,
  voter_id      UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  vote          vote_choice NOT NULL,
  voter_wallet  VARCHAR(58),                -- Algorand address used for on-chain vote
  txn_id        VARCHAR(52),                -- On-chain vote transaction ID
  created_at    TIMESTAMPTZ DEFAULT NOW(),

  -- One vote per user per dispute
  UNIQUE(dispute_id, voter_id)
);

CREATE INDEX idx_dao_votes_dispute ON dao_votes(dispute_id);
CREATE INDEX idx_dao_votes_voter ON dao_votes(voter_id);

-- ========================
-- 9. RATINGS TABLE [NEW] — From AlgoBounty reference
-- ========================

CREATE TABLE IF NOT EXISTS ratings (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bounty_id     UUID NOT NULL REFERENCES bounties(id) ON DELETE CASCADE,
  rater_id      UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,   -- Creator
  worker_id     UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,   -- Worker being rated
  stars         INT NOT NULL CHECK (stars >= 1 AND stars <= 5),
  comment       TEXT,
  created_at    TIMESTAMPTZ DEFAULT NOW(),

  -- One rating per bounty
  UNIQUE(bounty_id, rater_id)
);

CREATE INDEX idx_ratings_worker ON ratings(worker_id);
CREATE INDEX idx_ratings_stars ON ratings(stars);

-- ========================
-- 10. NOTIFICATIONS TABLE [NEW]
-- ========================

CREATE TABLE IF NOT EXISTS notifications (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  type          notification_type NOT NULL,
  title         VARCHAR(200) NOT NULL,
  message       TEXT NOT NULL,
  bounty_id     UUID REFERENCES bounties(id) ON DELETE SET NULL,
  is_read       BOOLEAN DEFAULT FALSE,
  metadata      JSONB,
  created_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_notifications_user ON notifications(user_id);
CREATE INDEX idx_notifications_unread ON notifications(user_id, is_read) WHERE is_read = FALSE;
CREATE INDEX idx_notifications_type ON notifications(type);

-- ========================
-- 11. TRANSACTION LOG (Immutable Audit Trail)
-- ========================

CREATE TABLE IF NOT EXISTS transaction_log (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bounty_id     UUID NOT NULL REFERENCES bounties(id) ON DELETE CASCADE,
  actor_id      UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  action        VARCHAR(50) NOT NULL,         -- create, lock, submit, approve, dispute, vote, rate, etc.
  txn_id        VARCHAR(52),                  -- Algorand transaction ID
  txn_note      VARCHAR(200),                 -- On-chain note field value [v2]
  amount_algo   DECIMAL(18,6),
  metadata      JSONB,
  created_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_txn_log_bounty ON transaction_log(bounty_id);
CREATE INDEX idx_txn_log_actor ON transaction_log(actor_id);
CREATE INDEX idx_txn_log_action ON transaction_log(action);

-- ========================
-- 12. ROW LEVEL SECURITY (RLS)
-- ========================

-- Enable RLS
ALTER TABLE profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE bounties ENABLE ROW LEVEL SECURITY;
ALTER TABLE submissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE disputes ENABLE ROW LEVEL SECURITY;
ALTER TABLE dao_votes ENABLE ROW LEVEL SECURITY;
ALTER TABLE ratings ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE transaction_log ENABLE ROW LEVEL SECURITY;

-- Profiles: Users can read all, update own
CREATE POLICY profiles_select ON profiles FOR SELECT USING (true);
CREATE POLICY profiles_update ON profiles FOR UPDATE USING (auth_user_id = current_setting('app.current_user_id', true));

-- Bounties: Public read, creators can insert/update own
CREATE POLICY bounties_select ON bounties FOR SELECT USING (true);
CREATE POLICY bounties_insert ON bounties FOR INSERT WITH CHECK (
  creator_id IN (SELECT id FROM profiles WHERE auth_user_id = current_setting('app.current_user_id', true))
);
CREATE POLICY bounties_update ON bounties FOR UPDATE USING (
  creator_id IN (SELECT id FROM profiles WHERE auth_user_id = current_setting('app.current_user_id', true))
);

-- Submissions: Public read for leaderboard, workers can insert
CREATE POLICY submissions_select ON submissions FOR SELECT USING (true);
CREATE POLICY submissions_insert ON submissions FOR INSERT WITH CHECK (
  worker_id IN (SELECT id FROM profiles WHERE auth_user_id = current_setting('app.current_user_id', true))
);

-- Disputes: Participants can read, either party can insert
CREATE POLICY disputes_select ON disputes FOR SELECT USING (true);
CREATE POLICY disputes_insert ON disputes FOR INSERT WITH CHECK (
  initiated_by IN (SELECT id FROM profiles WHERE auth_user_id = current_setting('app.current_user_id', true))
);

-- DAO Votes: Public read, voters can insert own
CREATE POLICY dao_votes_select ON dao_votes FOR SELECT USING (true);
CREATE POLICY dao_votes_insert ON dao_votes FOR INSERT WITH CHECK (
  voter_id IN (SELECT id FROM profiles WHERE auth_user_id = current_setting('app.current_user_id', true))
);

-- Ratings: Public read, raters can insert
CREATE POLICY ratings_select ON ratings FOR SELECT USING (true);
CREATE POLICY ratings_insert ON ratings FOR INSERT WITH CHECK (
  rater_id IN (SELECT id FROM profiles WHERE auth_user_id = current_setting('app.current_user_id', true))
);

-- Notifications: Users can only read own
CREATE POLICY notifications_select ON notifications FOR SELECT USING (
  user_id IN (SELECT id FROM profiles WHERE auth_user_id = current_setting('app.current_user_id', true))
);
CREATE POLICY notifications_update ON notifications FOR UPDATE USING (
  user_id IN (SELECT id FROM profiles WHERE auth_user_id = current_setting('app.current_user_id', true))
);

-- Transaction log: Public read, system insert (via service role)
CREATE POLICY txn_log_select ON transaction_log FOR SELECT USING (true);

-- ========================
-- 13. HELPER FUNCTIONS
-- ========================

-- Auto-update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER profiles_updated_at
  BEFORE UPDATE ON profiles
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER bounties_updated_at
  BEFORE UPDATE ON bounties
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Function to get submission count for a bounty
CREATE OR REPLACE FUNCTION get_submission_count(bounty_uuid UUID)
RETURNS INT AS $$
  SELECT COUNT(*)::INT FROM submissions WHERE bounty_id = bounty_uuid;
$$ LANGUAGE sql STABLE;

-- [NEW] Function to calculate average rating for a worker
CREATE OR REPLACE FUNCTION calc_avg_rating(worker_uuid UUID)
RETURNS DECIMAL(3,2) AS $$
  SELECT COALESCE(AVG(stars)::DECIMAL(3,2), 0) FROM ratings WHERE worker_id = worker_uuid;
$$ LANGUAGE sql STABLE;

-- [NEW] Function to update profile reputation after rating
CREATE OR REPLACE FUNCTION update_worker_reputation()
RETURNS TRIGGER AS $$
BEGIN
  UPDATE profiles SET
    avg_rating = calc_avg_rating(NEW.worker_id),
    total_ratings = (SELECT COUNT(*) FROM ratings WHERE worker_id = NEW.worker_id),
    reputation_score = (
      SELECT
        COALESCE(AVG(stars) * 20, 0)::INT  -- Convert 1-5 to 20-100
        + (SELECT total_bounties_completed * 5 FROM profiles WHERE id = NEW.worker_id)
      FROM ratings WHERE worker_id = NEW.worker_id
    )
  WHERE id = NEW.worker_id;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER rating_update_reputation
  AFTER INSERT ON ratings
  FOR EACH ROW EXECUTE FUNCTION update_worker_reputation();

-- [NEW] Function to get DAO vote tally for a dispute
CREATE OR REPLACE FUNCTION get_vote_tally(dispute_uuid UUID)
RETURNS TABLE(approve_count INT, reject_count INT, total_count INT) AS $$
  SELECT
    COUNT(*) FILTER (WHERE vote = 'approve')::INT as approve_count,
    COUNT(*) FILTER (WHERE vote = 'reject')::INT as reject_count,
    COUNT(*)::INT as total_count
  FROM dao_votes WHERE dispute_id = dispute_uuid;
$$ LANGUAGE sql STABLE;

-- [NEW] Leaderboard view
CREATE OR REPLACE VIEW leaderboard AS
SELECT
  p.id,
  p.username,
  p.display_name,
  p.avatar_url,
  p.wallet_address,
  p.reputation_score,
  p.total_bounties_completed,
  p.total_bounties_created,
  p.total_earnings_algo,
  p.avg_rating,
  p.streak_count,
  RANK() OVER (ORDER BY p.reputation_score DESC) as rank
FROM profiles p
WHERE p.total_bounties_completed > 0 OR p.total_bounties_created > 0
ORDER BY p.reputation_score DESC;

-- [NEW] Function to increment bounty view count
CREATE OR REPLACE FUNCTION increment_view_count(bounty_uuid UUID)
RETURNS VOID AS $$
  UPDATE bounties SET view_count = view_count + 1 WHERE id = bounty_uuid;
$$ LANGUAGE sql;

-- ============================================================
-- END OF SCHEMA
-- ============================================================
