# datastorectl

Declarative configuration for running datastores.

> **Work in progress.** datastorectl is under active development and not usable yet. The first release (v0.1.0) will target OpenSearch. See the scope and roadmap below.

## The Problem

Terraform and CloudFormation provision clusters. They don't manage what's inside them: ACL users, index lifecycle policies, eviction strategies, retention rules, role mappings. That config lives in ad-hoc scripts, manual CLI sessions, or brittle Ansible playbooks. When someone changes a security role through the OpenSearch dashboard at 2am, nobody finds out until something breaks.

datastorectl manages post-provisioning datastore configuration declaratively, the same way Terraform manages infrastructure.

## How It Works

Write config in DCL (Datastore Configuration Language), an HCL-inspired language purpose-built for datastore resources:

```hcl
context "prod-opensearch" {
  provider = opensearch
  endpoint = "https://search-prod.us-east-1.es.amazonaws.com"
  auth     = aws_sigv4("us-east-1")
}

opensearch_ism_policy "hot_warm_delete" {
  context = prod-opensearch

  description = "Move to warm after 7d, delete after 90d"

  state "hot" {
    actions = []
    transition {
      state    = warm
      condition = min_index_age("7d")
    }
  }

  state "warm" {
    actions = [
      { warm_migration = {} }
    ]
    transition {
      state    = delete
      condition = min_index_age("90d")
    }
  }

  state "delete" {
    actions = [
      { delete = {} }
    ]
  }
}

opensearch_role "log_reader" {
  context = prod-opensearch

  cluster_permissions = ["cluster_composite_ops_ro"]

  index_permissions {
    index_patterns  = ["logs-*"]
    allowed_actions = ["read"]
  }
}

opensearch_role_mapping "log_reader_mapping" {
  context = prod-opensearch

  role           = opensearch_role.log_reader
  backend_roles  = ["arn:aws:iam::123456789:role/log-reader"]
}
```

Then run three commands:

- `datastorectl validate`: parse and type-check DCL offline. No network calls.
- `datastorectl plan`: connect to the cluster, read live state, diff against declared state, show what would change.
- `datastorectl apply`: execute the changes. Supports `--dry-run`.

Plan output uses Terraform's conventions: `+` green for additions, `~` yellow for modifications, `-` red for deletions. Diffs are attribute-level by default.

## Stateless

No state file. datastorectl reads live state from the cluster on every run and computes diffs directly. No lock contention, no state corruption, no drift between a state file and reality.

This works because datastore configurations are identifiable by name (an ISM policy, a security role, an ACL user) and queryable through APIs. There's no equivalent of a randomly-assigned EC2 instance ID that forces you to track state externally.

## v0.1.0 Scope

OpenSearch as the first provider. 9 resource types:

1. ISM policies
2. Security roles
3. Role mappings
4. Internal users
5. Composable index templates
6. Component templates
7. Ingest pipelines
8. Cluster settings
9. Snapshot repositories

DCL layers 1 through 4: resource blocks with typed attributes, cross-resource references (`role = opensearch_role.log_reader`), the `secret()` built-in for credential resolution, and named connection contexts.

Three commands: `validate`, `plan`, `apply`.

## Roadmap

- v0.1.0: OpenSearch provider, core framework, DCL parser
- v0.5.0: Redis and MongoDB providers, variables/conditionals/iteration in DCL, CEL policy validation, Vault and AWS Secrets Manager backends
- v1.0.0: MySQL provider, gRPC external plugin protocol, GitOps integration (ArgoCD/Flux)

## License

Apache 2.0
