context "prod" {
  provider = opensearch
  endpoint = "https://prod:9200"
}

opensearch_role "reader" {
  cluster_permissions = ["read"]
}
