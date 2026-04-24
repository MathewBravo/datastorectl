#!/usr/bin/env bash
# capture.sh — regenerate parser fixtures from live MySQL containers.
#
# For each supported version, this script starts a fresh mysql:<version>
# container, creates a set of users and grants designed to cover every
# clause and scope the CREATE USER / GRANT parsers handle, dumps the
# SHOW CREATE USER / SHOW GRANTS output, and removes the container.
#
# After running this script, re-generate the .json expected-parsed
# outputs with:
#     go test ./providers/mysql/parse/... -update-fixtures
#
# That flow keeps fixtures reproducible and lets real-server drift
# surface as a test failure the next time someone runs without -update.
#
# Notes on the DDL:
#  - COMMENT and ATTRIBUTE cannot coexist on one user (server rejects).
#    They ship as separate fixtures.
#  - MySQL 8.4 disables mysql_native_password by default; we capture the
#    caching_sha2_password-shaped output, which is what production
#    users will see on 8.4 anyway.

set -u
set -o pipefail

VERSIONS=("8.0" "8.4")
ROOT_PW="capture_rootpw"
SCRATCH_PORT_START=33400

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

for i in "${!VERSIONS[@]}"; do
  version="${VERSIONS[$i]}"
  port=$((SCRATCH_PORT_START + i))
  name="dsctl-parse-capture-${version//./_}"
  users_dir="${here}/${version}/users"
  grants_dir="${here}/${version}/grants"
  mkdir -p "$users_dir" "$grants_dir"

  echo "=== MySQL ${version} on port ${port} ==="
  docker rm -f "$name" >/dev/null 2>&1 || true
  docker run -d --name "$name" \
    -e "MYSQL_ROOT_PASSWORD=${ROOT_PW}" \
    -p "${port}:3306" \
    "mysql:${version}" >/dev/null

  echo "Waiting for server to be ready..."
  until docker exec "$name" mysqladmin ping -h localhost -uroot -p"${ROOT_PW}" --silent 2>/dev/null; do
    sleep 2
  done

  run_sql() {
    docker exec -i "$name" mysql -uroot -p"${ROOT_PW}" 2>&1 >/dev/null
  }
  capture_show() {
    # $1 = SQL SHOW statement (quoted), $2 = output file
    # --raw disables the batch-mode escape transforms that would
    # otherwise double-escape SQL-escape sequences (\Z, \', etc.)
    # inside the DDL output and break parsing.
    echo "$1" \
      | docker exec -i "$name" mysql -uroot -p"${ROOT_PW}" -N -B --raw 2>/dev/null \
      | head -1 > "$2"
  }

  # --- Users ---
  # Each CREATE USER runs in its own statement so one bad clause doesn't
  # abort the rest of the corpus.
  run_sql <<SQL
CREATE USER 'basic_user'@'%' IDENTIFIED BY 'pw';
SQL
  run_sql <<SQL
CREATE USER 'user_limits'@'10.0.%' IDENTIFIED BY 'pw' WITH MAX_QUERIES_PER_HOUR 1000 MAX_CONNECTIONS_PER_HOUR 50 MAX_UPDATES_PER_HOUR 500 MAX_USER_CONNECTIONS 10;
SQL
  run_sql <<SQL
CREATE USER 'user_tls'@'%' IDENTIFIED BY 'pw' REQUIRE ISSUER '/CN=Internal CA' AND SUBJECT '/CN=client' AND CIPHER 'ECDHE-RSA-AES256-GCM-SHA384';
SQL
  run_sql <<SQL
CREATE USER 'user_ssl_flag'@'%' IDENTIFIED BY 'pw' REQUIRE SSL;
SQL
  run_sql <<SQL
CREATE USER 'user_policy'@'%' IDENTIFIED BY 'pw' PASSWORD EXPIRE INTERVAL 90 DAY PASSWORD HISTORY 5 PASSWORD REUSE INTERVAL 365 DAY PASSWORD REQUIRE CURRENT OPTIONAL FAILED_LOGIN_ATTEMPTS 3 PASSWORD_LOCK_TIME 1;
SQL
  run_sql <<SQL
CREATE USER 'user_locked'@'%' IDENTIFIED BY 'pw' ACCOUNT LOCK;
SQL
  run_sql <<SQL
CREATE USER 'user_comment'@'%' IDENTIFIED BY 'pw' COMMENT 'provisioned by datastorectl';
SQL
  run_sql <<SQL
CREATE USER 'user_attribute'@'%' IDENTIFIED BY 'pw' ATTRIBUTE '{"team":"dd"}';
SQL
  run_sql <<SQL
CREATE ROLE 'reader';
SQL

  capture_show "SHOW CREATE USER 'basic_user'@'%'"     "${users_dir}/basic_user.sql"
  capture_show "SHOW CREATE USER 'user_limits'@'10.0.%'" "${users_dir}/user_with_limits.sql"
  capture_show "SHOW CREATE USER 'user_tls'@'%'"       "${users_dir}/user_with_tls_require.sql"
  capture_show "SHOW CREATE USER 'user_ssl_flag'@'%'"  "${users_dir}/user_with_ssl_flag.sql"
  capture_show "SHOW CREATE USER 'user_policy'@'%'"    "${users_dir}/user_with_password_policy.sql"
  capture_show "SHOW CREATE USER 'user_locked'@'%'"    "${users_dir}/user_account_locked.sql"
  capture_show "SHOW CREATE USER 'user_comment'@'%'"   "${users_dir}/user_with_comment.sql"
  capture_show "SHOW CREATE USER 'user_attribute'@'%'" "${users_dir}/user_with_attribute.sql"
  capture_show "SHOW CREATE USER 'reader'@'%'"         "${users_dir}/role.sql"

  # --- Grants ---
  run_sql <<SQL
CREATE DATABASE appdb;
CREATE TABLE appdb.users (id INT);
GRANT PROCESS, REPLICATION CLIENT ON *.* TO 'basic_user'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON appdb.* TO 'basic_user'@'%';
GRANT ALL PRIVILEGES ON appdb.users TO 'basic_user'@'%' WITH GRANT OPTION;
SQL

  show_grants_output=$(echo "SHOW GRANTS FOR 'basic_user'@'%'" \
    | docker exec -i "$name" mysql -uroot -p"${ROOT_PW}" -N -B --raw 2>/dev/null \
    | grep -v "USAGE ON" || true)

  printf '%s\n' "$show_grants_output" > "${grants_dir}/multi_statement.sql"
  printf '%s\n' "$show_grants_output" | grep "ON \*\\.\\*" > "${grants_dir}/global_grant.sql" || true
  printf '%s\n' "$show_grants_output" | grep '`appdb`\.\*' > "${grants_dir}/schema_grant.sql" || true
  printf '%s\n' "$show_grants_output" | grep '`appdb`\.`users`' > "${grants_dir}/table_grant_with_option.sql" || true

  docker rm -f "$name" >/dev/null 2>&1
  echo "Captured ${version}."
done

echo ""
echo "Fixtures written under providers/mysql/parse/testdata/."
echo "Run: go test ./providers/mysql/parse/... -update-fixtures"
echo "to regenerate expected-parsed .json files, then commit."
