-- datastorectl-mysql bootstrap (read-write)
-- ------------------------------------------------------------
-- Creates the management account datastorectl expects when it runs
-- with auth = "password" or auth = "rds_iam" in plan/apply mode.
-- The grants here mirror ADR 0011's required-privileges list and
-- correspond to what the self-lockout guard (#177, #198) watches.
--
-- Trust model:
--   This user holds ALL PRIVILEGES ... WITH GRANT OPTION globally.
--   That is intentional — a tool that manages grants must itself hold
--   every privilege it may pass on. Puppet's mysql module operates
--   under the same constraint. Keep the password in a secrets store
--   and gate connections with REQUIRE SSL.
--
-- Before running:
--   1. Replace <REPLACE_ME> with a strong password (32+ chars).
--   2. Review the host pattern '%' — tighten it if your network
--      policy supports scoping (e.g. '10.0.0.0/8').
--   3. Apply against a fresh cluster or one where no prior
--      'datastorectl' account exists.

CREATE USER 'datastorectl'@'%'
  IDENTIFIED WITH caching_sha2_password BY '<REPLACE_ME>'
  REQUIRE SSL;

-- Read-side grants for Discover.
GRANT SELECT ON mysql.user         TO 'datastorectl'@'%';
GRANT SELECT ON mysql.db           TO 'datastorectl'@'%';
GRANT SELECT ON mysql.default_roles TO 'datastorectl'@'%';
GRANT SELECT ON mysql.role_edges   TO 'datastorectl'@'%';

-- Write-side grants for Apply. ALL PRIVILEGES covers CREATE USER,
-- CREATE/DROP ROLE, CREATE/DROP SCHEMA, GRANT/REVOKE at every scope.
-- SYSTEM_USER is the 8.0+ dynamic privilege needed to manage other
-- accounts that themselves hold SYSTEM_USER.
GRANT ALL PRIVILEGES ON *.* TO 'datastorectl'@'%' WITH GRANT OPTION;
GRANT SYSTEM_USER ON *.* TO 'datastorectl'@'%';

FLUSH PRIVILEGES;
