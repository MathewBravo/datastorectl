context "demo" {
  provider = mysql
  version  = "8.4"
  endpoint = "localhost:3306"
  auth     = "password"
  username = "datastorectl"
  password = secret("env", "DATASTORECTL_MYSQL_PASSWORD")
  tls      = "skip-verify"
}

# --- Schema ---

mysql_database "appdb" {
  context   = demo
  charset   = "utf8mb4"
  collation = "utf8mb4_0900_ai_ci"
}

# --- Users ---

# App user. Cleartext password resolved from env via secret();
# the provider rehashes against the server-chosen salt to diff.
mysql_user "app" {
  context  = demo
  user     = "app"
  host     = "%"
  password = secret("env", "DATASTORECTL_MYSQL_APP_PW")
}

# Ops user. Second credential resolved from its own env var. Real
# deployments would source distinct passwords per user from a secret
# store via an appropriate secret() backend.
mysql_user "ops" {
  context  = demo
  user     = "ops"
  host     = "%"
  password = secret("env", "DATASTORECTL_MYSQL_OPS_PW")
}

# Note on password_hash: the provider also supports
#     password_hash = secret("env", "...")
# for users whose mysql_native_password hashes already live in
# Hiera or a secret store (the Puppet migration path). We don't
# demonstrate it here because mysql_native_password is disabled by
# default on mysql:8.4. See providers/mysql/README.md for that form.

# --- Roles ---

mysql_role "reader" {
  context = demo
}

# --- Grants ---

# Global monitoring access for the reader role.
mysql_grant "reader_monitoring" {
  context    = demo
  user       = "reader"
  host       = "%"
  database   = "*"
  table      = "*"
  privileges = ["PROCESS", "REPLICATION CLIENT"]
}

# Schema-level data grant for the app user.
mysql_grant "app_schema" {
  context    = demo
  user       = "app"
  host       = "%"
  database   = "appdb"
  table      = "*"
  privileges = ["SELECT", "INSERT", "UPDATE", "DELETE"]
}

# Schema-level grant with GRANT OPTION. Ops can further delegate
# SELECT on appdb to other users. Table-level scope works the same
# way — drop `table = "*"` for `table = "events"` (the table must
# exist before the grant applies).
mysql_grant "ops_appdb_readable" {
  context      = demo
  user         = "ops"
  host         = "%"
  database     = "appdb"
  table        = "*"
  privileges   = ["SELECT"]
  grant_option = true
}

# --- Self-lockout demo note ---
#
# The management user (datastorectl) authenticates via the context
# above. It holds ALL PRIVILEGES WITH GRANT OPTION globally, plus
# mysql.* SELECTs, none of which are declared as mysql_grant blocks
# here. That is deliberate — running `apply --prune` against this
# config would attempt to revoke every one of those grants, which
# the self-lockout guard catches and refuses with a clear reason.
#
# Try:   ./datastorectl plan --prune testdata/showcase-mysql/resources.dcl
