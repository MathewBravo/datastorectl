context "demo" {
  provider        = opensearch
  endpoint        = "https://localhost:9200"
  auth            = "basic"
  username        = "admin"
  password        = "myStrongPassword123!"
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

opensearch_role_mapping "log_reader" {
  context       = demo
  backend_roles = ["arn:aws:iam::123456789012:role/log-reader"]
  description   = "Map IAM role to log_reader"
}

# Bootstrap role mappings shipped by OpenSearch's security plugin.
# Declared verbatim so prune sees them as no-ops instead of deletes.
# Without these declarations, apply would delete all_access (locking
# out admin) and the five other bootstrap mappings.

opensearch_role_mapping "all_access" {
  context       = demo
  backend_roles = ["admin"]
  description   = "Maps admin to all_access"
}

opensearch_role_mapping "readall" {
  context       = demo
  backend_roles = ["readall"]
}

opensearch_role_mapping "own_index" {
  context     = demo
  users       = ["*"]
  description = "Allow full access to an index named like the username"
}

opensearch_role_mapping "logstash" {
  context       = demo
  backend_roles = ["logstash"]
}

opensearch_role_mapping "kibana_user" {
  context       = demo
  backend_roles = ["kibanauser"]
  description   = "Maps kibanauser to kibana_user"
}

opensearch_role_mapping "manage_snapshots" {
  context       = demo
  backend_roles = ["snapshotrestore"]
}

opensearch_ism_policy "hot_delete" {
  context = demo

  default_state = "hot"
  description   = "Delete indices after 30 days"

  states = [
    {
      name    = "hot"
      actions = []
      transitions = [
        {
          state_name = "delete"
          conditions = {
            min_index_age = "30d"
          }
        }
      ]
    },
    {
      name = "delete"
      actions = [
        { delete = {} }
      ]
      transitions = []
    }
  ]
}
