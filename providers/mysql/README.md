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
- port `3308` exposed on the host (container port stays `3306`; the
  host-side remap avoids conflicts with a locally-installed MySQL or
  another dev project's `mysql` container)

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

## Authenticating with AWS RDS IAM

The provider supports `auth = "rds_iam"` for AWS RDS / Aurora clusters
that have IAM database authentication enabled. Configuration:

```hcl
context "prod_rds" {
  provider = mysql
  version  = "8.4"
  endpoint = "prod.cluster-abc.us-east-1.rds.amazonaws.com:3306"
  auth     = "rds_iam"
  username = "datastorectl_iam"
  region   = "us-east-1"

  # optional — pin to a specific shared-config profile
  # aws_profile = "prod"

  tls      = "required"   # mandatory for rds_iam
}
```

The caller's AWS identity (from env vars, `~/.aws/credentials`, an
EC2/EKS role, or the specified profile) must have an IAM policy
permitting `rds-db:connect` on the target DB resource:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": "rds-db:connect",
    "Resource": "arn:aws:rds-db:us-east-1:<account>:dbuser:<resource-id>/datastorectl_iam"
  }]
}
```

The database user itself must be created with IAM authentication
enabled on the server:

```sql
CREATE USER 'datastorectl_iam'@'%' IDENTIFIED WITH AWSAuthenticationPlugin AS 'RDS';
```

Tokens are generated via the AWS SDK at Configure time with ~15-minute
validity. Since datastorectl is a short-lived CLI, one token per
invocation is sufficient — no refresh logic is needed.

Integration tests for the rds_iam path require a reachable RDS
endpoint and valid AWS credentials. Set
`DATASTORECTL_MYSQL_TEST_RDS_ENDPOINT` and ensure the calling identity
has `rds-db:connect` on the target. Without these, rds_iam integration
tests skip; unit tests for the validation paths still run.
