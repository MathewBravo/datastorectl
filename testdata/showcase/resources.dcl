context "local" {
  provider = opensearch
  endpoint = "https://localhost:9200"
  auth     = "basic"
  username = "admin"
  password = "myStrongPassword123!"
}

opensearch_role "plan_test_reader" {
  context = local
  cluster_permissions = ["cluster_monitor"]
}
