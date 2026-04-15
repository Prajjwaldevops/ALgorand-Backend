-- ============================================================
-- BountyVault — Fix dao_votes.vote column enum type
-- 
-- Problem: The dao_votes table was created with the old 
-- 'vote_choice' enum ('approve', 'reject'), but the backend
-- sends 'creator' or 'freelancer' (from dao_vote_choice enum).
--
-- This migration:
-- 1. Ensures dao_vote_choice enum exists
-- 2. Alters the dao_votes.vote column to use dao_vote_choice
-- 3. Adds vote_txn_id column if missing
-- ============================================================

-- Step 1: Ensure dao_vote_choice enum exists
DO $$ BEGIN
  CREATE TYPE dao_vote_choice AS ENUM ('creator', 'freelancer');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Step 2: Alter column type from vote_choice to dao_vote_choice
-- We need to drop the old column and recreate it since PostgreSQL
-- doesn't allow direct enum-to-enum ALTER TYPE changes easily.
-- First check if the column uses the old type.
DO $$
BEGIN
  -- Only alter if the column currently uses vote_choice (not dao_vote_choice)
  IF EXISTS (
    SELECT 1 FROM information_schema.columns 
    WHERE table_name = 'dao_votes' 
      AND column_name = 'vote' 
      AND udt_name = 'vote_choice'
  ) THEN
    -- Drop any existing rows with old enum values (approve/reject)
    -- since they can't be converted to the new enum
    DELETE FROM dao_votes WHERE vote::text NOT IN ('creator', 'freelancer');
    
    -- Alter the column: convert via text intermediary
    ALTER TABLE dao_votes 
      ALTER COLUMN vote TYPE dao_vote_choice 
      USING vote::text::dao_vote_choice;
    
    RAISE NOTICE 'dao_votes.vote column converted from vote_choice to dao_vote_choice';
  ELSE
    RAISE NOTICE 'dao_votes.vote column already uses correct type, skipping';
  END IF;
END $$;

-- Step 3: Ensure vote_txn_id column exists (some schemas have txn_id instead)
ALTER TABLE dao_votes ADD COLUMN IF NOT EXISTS vote_txn_id VARCHAR(52);

-- Step 4: Ensure voted_at column exists
ALTER TABLE dao_votes ADD COLUMN IF NOT EXISTS voted_at TIMESTAMPTZ DEFAULT NOW();

-- ============================================================
-- END OF MIGRATION
-- ============================================================
