context "prod" {
  provider = opensearch
  endpoint = "https://prod:9200"
  auth     = "basic"
  username = "admin"
  password = "secret"
}

context "staging" {
  provider = opensearch
  endpoint = "https://staging:9200"
  auth     = "basic"
  username = "admin"
  password = "secret"
}
