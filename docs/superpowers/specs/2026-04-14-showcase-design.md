# End-to-End Showcase Design

## Goal

Enable a manual interactive demo of the full datastorectl lifecycle: validate → plan → apply → plan-again, running against a fresh Docker OpenSearch cluster.

## Pieces

### 1. `tls_skip_verify` in the OpenSearch provider

**Problem:** The Docker OpenSearch cluster uses a self-signed TLS cert. The provider's `NewClient` doesn't support skipping TLS verification (only the test helpers do). Self-hosted users with internal CAs or dev clusters will hit the same issue.

**Change:** Add an optional `tls_skip_verify` boolean attribute to the provider config. When `true`, the HTTP client sets `InsecureSkipVerify` on its TLS config.

**Files:**
- `providers/opensearch/client.go` — `NewClient` and `NewSigV4Client` accept a `tlsSkipVerify bool` parameter
- `providers/opensearch/provider.go` — `configureBasicAuth` and `configureSigV4` read the attribute and pass it through

**Not included:** `ca_cert` support (follow-up ticket for internal CA bundles).

### 2. Sample DCL file

A single file at `testdata/showcase/resources.dcl` with:
- An inline context block targeting `https://localhost:9200` with basic auth and `tls_skip_verify = true`
- 3 resources that exercise the dependency chain and multiple resource types:
  - `opensearch_role "log_reader"` — a role with cluster and index permissions
  - `opensearch_role_mapping "log_reader"` — maps a backend role ARN to the role above
  - `opensearch_ism_policy "hot_delete"` — a simple 2-state ISM policy (hot → delete)

These are chosen to show: colored diffs for different resource types, cross-resource type ordering (role before mapping), and a non-trivial nested body (ISM policy states).

### 3. Helper script

`showcase.sh` at the repo root with two subcommands:
- `./showcase.sh up` — runs `docker compose up -d`, waits for the cluster to be healthy (poll `/_cluster/health`), and builds the `datastorectl` binary
- `./showcase.sh down` — runs `docker compose down`

The user runs the four datastorectl commands manually between `up` and `down`.

### 4. README section

A short section in the repo README (or a `SHOWCASE.md`) documenting:
```bash
./showcase.sh up
./datastorectl validate testdata/showcase/resources.dcl
./datastorectl plan testdata/showcase/resources.dcl
./datastorectl apply testdata/showcase/resources.dcl
./datastorectl plan testdata/showcase/resources.dcl   # "No changes."
./showcase.sh down
```

## What this does NOT include

- `ca_cert` provider option (separate follow-up)
- Automated Go integration test for the CLI
- asciinema recording (can be produced from the script later)
- Any changes to the engine, config, or output modules
