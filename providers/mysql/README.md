# mysql provider tests

Unit tests (validation, helpers, future hash routines, future DDL
parser) run without any external dependency:

```
go test ./providers/mysql/...
```

Integration tests require a running MySQL 8.4 server. When none is
reachable, integration tests skip automatically with a notice on
stderr. Unit tests continue to run.

## Run against the bundled Docker Compose fixture

```
docker compose -f docker-compose.mysql.yml up -d
go test ./providers/mysql/...
docker compose -f docker-compose.mysql.yml down
```

The fixture brings up a single `mysql:8.4` node with:
- root password `datastorectl`
- root accessible from any host (`MYSQL_ROOT_HOST=%`)
- port `3306` exposed on the host

TLS uses the server-generated self-signed certs that ship with the
image. Integration tests connect with `tls = "skip-verify"` to
tolerate them.

## Run against a custom endpoint

```
export DATASTORECTL_MYSQL_TEST_ENDPOINT=host.example.com:3306
go test ./providers/mysql/...
```

The same `root` / `datastorectl` credentials are used. If your
endpoint requires different credentials, that's a gap — open an
issue.

## Test helpers

- `newTestClient(t)` — opens a `*Client` against the integration
  cluster; registers a cleanup that closes the pool. Skips t if no
  cluster is reachable.
- `createTestDatabase(t, c, name)` — creates a fresh schema and
  registers a cleanup that drops it.
- `createTestUser(t, c, user, host, password)` — creates a user
  with `caching_sha2_password` and registers a cleanup that drops
  it.

Handler tests in later phases build on these.
