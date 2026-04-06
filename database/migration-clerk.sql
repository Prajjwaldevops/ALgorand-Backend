-- Clerk Database Migration

-- 1. Remove the old Better Auth function.
-- Note: If the "user" table still exists, you can manually drop its trigger if needed.
DROP FUNCTION IF EXISTS public.handle_new_user() CASCADE;

-- 2. Add the clerk_id column to the profiles table
ALTER TABLE public.profiles
ADD COLUMN IF NOT EXISTS clerk_id TEXT UNIQUE;

-- 3. (Optional) Run any other data cleanup if necessary

-- 4. Set a safe fallback for existing rows if needed (Optional)
-- UPDATE public.profiles SET clerk_id = 'legacy_' || id::text WHERE clerk_id IS NULL;
