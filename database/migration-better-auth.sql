-- Better Auth Database Migration

-- 1. Modify existing "user" table to match Better Auth schema
ALTER TABLE "user" 
  RENAME COLUMN email_verified TO "emailVerified";
ALTER TABLE "user" 
  RENAME COLUMN created_at TO "createdAt";
ALTER TABLE "user" 
  RENAME COLUMN updated_at TO "updatedAt";

ALTER TABLE "user" 
  ADD COLUMN IF NOT EXISTS "name" TEXT NOT NULL DEFAULT 'User',
  ADD COLUMN IF NOT EXISTS "image" TEXT;

ALTER TABLE "user" ALTER COLUMN "name" DROP DEFAULT;

-- Better Auth stores passwords in the 'account' table, so we can drop it from 'user'
-- (Do this only if you don't care about migrating existing passwords)
ALTER TABLE "user" DROP COLUMN IF EXISTS password_hash;


-- 2. Create 'session' table needed by Better Auth
CREATE TABLE IF NOT EXISTS "session" (
  "id" TEXT PRIMARY KEY,
  "expiresAt" TIMESTAMP NOT NULL,
  "token" TEXT NOT NULL UNIQUE,
  "createdAt" TIMESTAMP NOT NULL,
  "updatedAt" TIMESTAMP NOT NULL,
  "ipAddress" TEXT,
  "userAgent" TEXT,
  "userId" TEXT NOT NULL REFERENCES "user"("id") ON DELETE CASCADE
);

-- 3. Create 'account' table needed by Better Auth
CREATE TABLE IF NOT EXISTS "account" (
  "id" TEXT PRIMARY KEY,
  "accountId" TEXT NOT NULL,
  "providerId" TEXT NOT NULL,
  "userId" TEXT NOT NULL REFERENCES "user"("id") ON DELETE CASCADE,
  "accessToken" TEXT,
  "refreshToken" TEXT,
  "idToken" TEXT,
  "accessTokenExpiresAt" TIMESTAMP,
  "refreshTokenExpiresAt" TIMESTAMP,
  "scope" TEXT,
  "password" TEXT,
  "createdAt" TIMESTAMP NOT NULL,
  "updatedAt" TIMESTAMP NOT NULL
);

-- 4. Create 'verification' table for forgot password / email flows
CREATE TABLE IF NOT EXISTS "verification" (
  "id" TEXT PRIMARY KEY,
  "identifier" TEXT NOT NULL,
  "value" TEXT NOT NULL,
  "expiresAt" TIMESTAMP NOT NULL,
  "createdAt" TIMESTAMP,
  "updatedAt" TIMESTAMP
);

-- 5. Trigger to automatically create a Profile for a new Better Auth user
-- This replaces the Go backend manual insertion
CREATE OR REPLACE FUNCTION public.handle_new_user() 
RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO public.profiles (auth_user_id, username, display_name, role)
  VALUES (
    NEW.id,
    -- Generate a random username or use the name 
    LOWER(REGEXP_REPLACE(NEW.name, '\s+', '', 'g')) || '_' || substr(md5(random()::text), 1, 6),
    NEW.name,
    'worker' -- Default role, can be updated later
  );
  RETURN NEW;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Drop trigger if exists
DROP TRIGGER IF EXISTS on_auth_user_created ON "user";

-- Create trigger
CREATE TRIGGER on_auth_user_created
  AFTER INSERT ON "user"
  FOR EACH ROW EXECUTE PROCEDURE public.handle_new_user();
