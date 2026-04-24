# datastorectl

Declarative configuration for day-2 datastore operations. Terraform-style workflow for the stuff Terraform doesn't manage: ISM policies, security roles, role mappings, index templates, ingest pipelines, cluster settings.

> **Status:** v0.1.0 release candidate. The core framework and the OpenSearch provider are feature-complete and tested. Release plumbing (goreleaser, Homebrew tap) is the next milestone.

## The problem

Terraform and CloudFormation provision clusters. They don't manage what's inside them. ACL users, lifecycle policies, retention rules, role mappings live in ad-hoc scripts, manual CLI sessions, or brittle Ansible playbooks. When someone edits a security role through the OpenSearch dashboard at 2am, nobody finds out until something breaks.

datastorectl manages that layer the same way Terraform manages infrastructure: you declare what you want in code, the tool reads the live cluster, shows you the diff, and applies the changes.

## A taste of DCL

DCL (Datastore Configuration Language) is HCL-inspired, purpose-built for datastore resources. Connection contexts and resources use the same syntax.

```hcl
context "demo" {
  provider        = opensearch
  endpoint        = "https://localhost:9200"
  auth            = "basic"
  username        = "admin"
  password        = secret("env", "OPENSEARCH_PASSWORD")
  tls_skip_verify = true
}

opensearch_role "log_reader" {
  context = demo

  cluster_permissions = ["cluster_monitor"]

  index_permissions = [
    {
      index_patterns  = ["logs-*"]
      allowed_actions = ["read", "search"]
    }
  ]
}

opensearch_ism_policy "hot_delete" {
  context       = demo
  default_state = "hot"
  description   = "Delete indices after 30 days"

  states = [
    {
      name    = "hot"
      actions = []
      transitions = [
        { state_name = "delete", conditions = { min_index_age = "30d" } }
      ]
    },
    {
      name    = "delete"
      actions = [{ delete = {} }]
      transitions = []
    }
  ]
}
```

Or a MySQL grant set:

```hcl
context "demo" {
  provider = mysql
  version  = "8.4"
  endpoint = "localhost:3308"
  auth     = "password"
  username = "datastorectl"
  password = secret("env", "DATASTORECTL_MYSQL_PASSWORD")
  tls      = "skip-verify"
}

mysql_database "appdb" {
  context   = demo
  charset   = "utf8mb4"
  collation = "utf8mb4_0900_ai_ci"
}

mysql_user "app" {
  context  = demo
  user     = "app"
  host     = "%"
  password = secret("env", "DATASTORECTL_MYSQL_APP_PW")
}

mysql_grant "app_schema" {
  context    = demo
  user       = "app"
  host       = "%"
  database   = "appdb"
  table      = "*"
  privileges = ["SELECT", "INSERT", "UPDATE", "DELETE"]
}
```

## The three commands

```
datastorectl validate [path]   # parse and type-check offline; no network calls
datastorectl plan [path]       # read live state, compute diff, print it
datastorectl apply [path]      # execute the diff
```

`path` is a file or a directory. Directories are scanned recursively for `.dcl` files.

Plan output uses Terraform's conventions: `+` green for additions, `~` yellow for modifications, `-` red for deletions. Diffs are attribute-level by default. Use `--verbose` for full before/after, or `--output json` for structured output.

### Additive by default, opt-in deletes

`plan` and `apply` only create and update by default. Deletions are filtered out unless you pass `--prune`. This is a deliberate safety rail: a new user pointing at a shared cluster won't accidentally remove resources they didn't declare.

```
datastorectl plan              # shows creates and updates; deletes listed as "unmanaged"
datastorectl plan --prune      # include deletes
datastorectl apply --prune     # actually delete the unmanaged resources
```

### Self-lockout protection

Some deletes would revoke the caller's own access. The OpenSearch provider detects when a `--prune` pass would delete the role mapping or internal user that owns the current credentials, and refuses. Override with `--allow-self-lockout` if you know what you're doing.

## Quickstart

Local OpenSearch via Docker Compose:

```
./showcase.sh up                                                # start cluster + build binary
./datastorectl validate testdata/showcase/resources.dcl
./datastorectl plan     testdata/showcase/resources.dcl
./datastorectl apply    testdata/showcase/resources.dcl
./datastorectl plan     testdata/showcase/resources.dcl         # should report no changes
./showcase.sh down
```

Local MySQL via Docker Compose:

```
./showcase-mysql.sh up                                          # start MySQL + run bootstrap + build binary
export DATASTORECTL_MYSQL_PASSWORD=...                          # printed by showcase-mysql.sh up
export DATASTORECTL_MYSQL_APP_PW=...
export DATASTORECTL_MYSQL_OPS_PW=...
./datastorectl validate testdata/showcase-mysql/resources.dcl
./datastorectl plan     testdata/showcase-mysql/resources.dcl
./datastorectl apply    testdata/showcase-mysql/resources.dcl
./datastorectl plan     testdata/showcase-mysql/resources.dcl   # should report no changes
./showcase-mysql.sh down
```

Build from source:

```
go build -o datastorectl ./cmd/datastorectl
```

## Stateless

No state file. datastorectl reads live state from the cluster on every run and computes diffs directly. No lock contention, no state corruption.

This works because datastore configurations are identifiable by name. ISM policies, security roles, internal users — each has a stable name that's queryable through the API. There's no equivalent of a randomly-assigned EC2 instance ID that forces you to track state externally.

## Configuration

Connection contexts live separately from resource declarations. Put them in `~/.datastorectl/config.dcl`:

```hcl
context "prod" {
  provider = opensearch
  endpoint = "https://search-prod.us-east-1.es.amazonaws.com"
  auth     = "aws_sigv4"
  region   = "us-east-1"
}

context "staging" {
  provider = opensearch
  endpoint = "https://staging:9200"
  auth     = "basic"
  username = "admin"
  password = secret("env", "STAGING_PASSWORD")
}
```

Resource files reference a context by name (`context = prod`). The context config stays local; resource files are safe to commit. Secrets resolve at runtime via `secret("env", "VAR_NAME")` so credentials never sit in files.

When a directory contains resources targeting multiple contexts, pass `--context <name>` to pick one. Safety rail: without the flag, datastorectl refuses to run.

### MySQL provider

The MySQL provider supports two auth modes, TLS with configurable strictness, and three password declaration shapes on `mysql_user`.

**`auth = "password"`** — self-managed MySQL 8.0 / 8.4.

```hcl
context "prod" {
  provider = mysql
  version  = "8.4"
  endpoint = "prod.example.com:3306"
  auth     = "password"
  username = "datastorectl"
  password = secret("env", "DATASTORECTL_MYSQL_PASSWORD")
  tls      = "required"
  # tls_ca   = "/etc/ssl/rds-combined-ca-bundle.pem"  # optional custom CA
  # tls_cert = "/etc/ssl/client.pem"                  # optional client cert
  # tls_key  = "/etc/ssl/client.key"                  # optional client key
}
```

**`auth = "rds_iam"`** — managed AWS RDS / Aurora with IAM token auth.

```hcl
context "prod" {
  provider    = mysql
  version     = "8.4"
  endpoint    = "prod-cluster.cluster-xyz.us-east-1.rds.amazonaws.com:3306"
  auth        = "rds_iam"
  username    = "datastorectl"
  region      = "us-east-1"
  aws_profile = "prod"        # optional; defaults to the ambient AWS credential chain
  tls         = "required"    # cannot be "disabled" for rds_iam
  # tls_ca    = "/etc/ssl/rds-combined-ca-bundle.pem" # optional custom CA
}
```

**`version`** — required, either `"8.0"` or `"8.4"`. Must match the server's major.minor; patch and vendor suffixes are tolerated.

**TLS** — `tls` is one of:

- `"required"` (default) — verified TLS against the server's CA chain.
- `"skip-verify"` — TLS in transit, certificate chain not verified. Useful for self-signed dev clusters; never use in production.
- `"disabled"` — plaintext. Rejected for `auth = "rds_iam"`.

`tls_ca`, `tls_cert`, `tls_key` are optional paths to a custom CA bundle and/or client certificate for mutual TLS.

**Bootstrap SQL scripts** — `providers/mysql/bootstrap/` ships two SQL scripts that create the management account datastorectl connects as:

- `bootstrap-readwrite.sql` creates `datastorectl@%` with `ALL PRIVILEGES ... WITH GRANT OPTION` globally plus `SELECT` on the relevant `mysql.*` schema tables. Use for plan/apply workflows. The broad grant is a category constraint: a grant-management tool must itself hold every privilege it may pass on.
- `bootstrap-readonly.sql` creates `datastorectl-ro@%` with only the `mysql.*` `SELECT` grants needed for Discover. Use for drift-detection `plan` runs in CI or reviewer access. `apply` under this user will fail at the first write with a MySQL permission error.

Both scripts enforce `REQUIRE SSL` on the account.

**Password declaration shapes on `mysql_user`** — three forms, pick one per user:

```hcl
# 1. Cleartext. Provider rehashes against the server-chosen salt
#    to diff. The secret() lookup keeps the password out of files.
mysql_user "app" {
  context  = prod
  user     = "app"
  host     = "%"
  password = secret("env", "APP_PW")
}

# 2. Pre-hashed. Byte-compared against the server's stored hash.
#    Migration path for mysql_native_password hashes already sitting
#    in Hiera, Puppet, or a secret store.
mysql_user "legacy" {
  context       = prod
  user          = "legacy"
  host          = "%"
  password_hash = secret("env", "LEGACY_PW_HASH")
}

# 3. AWS IAM. No password field; the server delegates auth to AWS.
mysql_user "svc" {
  context     = prod
  user        = "svc"
  host        = "%"
  auth_plugin = "aws_iam"
}
```

## v0.1.0 scope

OpenSearch as the first provider. 9 resource types:

- `opensearch_ism_policy`
- `opensearch_role`
- `opensearch_role_mapping`
- `opensearch_internal_user`
- `opensearch_composable_index_template`
- `opensearch_component_template`
- `opensearch_ingest_pipeline`
- `opensearch_cluster_setting`
- `opensearch_snapshot_repository`

Auth: basic (self-hosted) and AWS SigV4 (managed OpenSearch Service).

DCL layers 1 through 4: resource blocks with typed attributes, cross-resource references (`role = opensearch_role.log_reader`), the `secret()` built-in for credential resolution, and named connection contexts.

MySQL as the second provider. 4 resource types:

- `mysql_database`
- `mysql_user`
- `mysql_role`
- `mysql_grant`

Auth: password (self-managed MySQL 8.0 / 8.4) and AWS RDS IAM (managed RDS / Aurora).

## Architecture

Ten modules, each with a narrow job:

| Module | Responsibility |
|--------|----------------|
| `dcl` | Hand-written recursive-descent parser. Lexer, AST, diagnostics. No HCL dependency. |
| `provider` | Provider interface, resource types, schema system. |
| `engine` | Discover, diff, dependency graph, parallel executor. Owns diffing so providers don't have to. |
| `providers/opensearch` | OpenSearch provider. HTTP client with basic + SigV4 auth. |
| `providers/mysql` | MySQL provider. database/user/role/grant handlers; diffing, self-lockout guard. |
| `providers/mysql/auth` | Auth dispatch — caching_sha2_password, mysql_native_password, aws_iam plugins. |
| `providers/mysql/parse` | GRANT statement parser (server GRANT strings → normalized privilege sets for diffing). |
| `config` | Loads `~/.datastorectl/config.dcl`. Context resolution. Secret lookups. |
| `output` | Terraform-style colored diffs, JSON output, diagnostic formatting. |
| `cmd/datastorectl` | Cobra CLI. Thin orchestration layer. |

Independent resources apply in parallel. Dependency chains stop on first error. Per-resource result reporting so you know exactly what succeeded and what failed.

## License

Apache 2.0.
