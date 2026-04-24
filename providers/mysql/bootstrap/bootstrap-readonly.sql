-- datastorectl-mysql bootstrap (read-only)
-- ------------------------------------------------------------
-- Creates a read-only management account for plan-only workflows.
-- Use this when:
--   - CI runs a drift-detection `datastorectl plan` and you want to
--     grant it the minimum privileges required.
--   - A code reviewer runs plan against a preview cluster without
--     elevation.
--   - You want to see what datastorectl would change before granting
--     the full read-write account.
--
-- This account can run validate and plan but NOT apply. Any apply
-- will fail at SQL-execution time with a permission error.
--
-- Trust model:
--   Narrow, read-side only. SELECT on the mysql schema tables the
--   provider needs for Discover, plus REQUIRE SSL to prevent
--   credential leakage. No grant option, no CREATE USER, no write
--   privileges anywhere.
--
-- Before running:
--   1. Replace <REPLACE_ME> with a strong password (32+ chars).
--   2. Review the host pattern '%' as with the read-write variant.
--
-- This script is idempotent; see the note in bootstrap-readwrite.sql
-- for the CREATE-then-ALTER pattern rationale.

CREATE USER IF NOT EXISTS 'datastorectl-ro'@'%'
  IDENTIFIED WITH caching_sha2_password BY '<REPLACE_ME>'
  REQUIRE SSL;

ALTER USER IF EXISTS 'datastorectl-ro'@'%'
  IDENTIFIED WITH caching_sha2_password BY '<REPLACE_ME>'
  REQUIRE SSL;

-- Read-side grants only. Mirror of the read-side grants in
-- bootstrap-readwrite.sql.
GRANT SELECT ON mysql.user          TO 'datastorectl-ro'@'%';
GRANT SELECT ON mysql.db            TO 'datastorectl-ro'@'%';
GRANT SELECT ON mysql.default_roles TO 'datastorectl-ro'@'%';
GRANT SELECT ON mysql.role_edges    TO 'datastorectl-ro'@'%';

FLUSH PRIVILEGES;
