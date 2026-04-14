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

  index_permissions {
    index_patterns  = ["logs-*"]
    allowed_actions = ["read", "search"]
  }
}

opensearch_role_mapping "log_reader" {
  context       = demo
  backend_roles = ["arn:aws:iam::123456789012:role/log-reader"]
  description   = "Map IAM role to log_reader"
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
