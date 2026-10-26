PRAGMA foreign_keys=OFF;
BEGIN;
ALTER TABLE users
DROP COLUMN approval_pending;
COMMIT;
PRAGMA foreign_keys=ON;
