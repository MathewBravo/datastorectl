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

## Roadmap

TBD once v0.1.0 is out.

## Architecture

Seven modules, each with a narrow job:

| Module | Responsibility |
|--------|----------------|
| `dcl` | Hand-written recursive-descent parser. Lexer, AST, diagnostics. No HCL dependency. |
| `provider` | Provider interface, resource types, schema system. |
| `engine` | Discover, diff, dependency graph, parallel executor. Owns diffing so providers don't have to. |
| `providers/opensearch` | OpenSearch provider. HTTP client with basic + SigV4 auth. |
| `config` | Loads `~/.datastorectl/config.dcl`. Context resolution. Secret lookups. |
| `output` | Terraform-style colored diffs, JSON output, diagnostic formatting. |
| `cmd/datastorectl` | Cobra CLI. Thin orchestration layer. |

Independent resources apply in parallel. Dependency chains stop on first error. Per-resource result reporting so you know exactly what succeeded and what failed.

## License

Apache 2.0.
