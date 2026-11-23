DROP INDEX IF EXISTS ix_companies_listed;
ALTER TABLE companies DROP CONSTRAINT IF EXISTS ck_companies_deleted;
ALTER TABLE companies DROP COLUMN IF EXISTS deleted_at;
