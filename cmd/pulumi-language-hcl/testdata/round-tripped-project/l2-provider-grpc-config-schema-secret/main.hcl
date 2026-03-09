terraform {
  required_providers {
    config-grpc = {
      source  = "pulumi/config-grpc"
      version = "1.0.0"
    }
  }
}

resource "pulumi_providers_config-grpc" "config_grpc_provider" {
  secret_string1      = "SECRET"
  secret_int1         = 16
  secret_num1         = 123456.789
  secret_bool1        = true
  list_secret_string1 = ["SECRET", "SECRET2"]
  map_secret_string1 = {
    "key1" = "SECRET"
    "key2" = "SECRET2"
  }
  obj_secret_string1 = {
    secret_x = "SECRET"
  }
}
resource "config-grpc_configfetcher" "config" {
  provider = pulumi_providers_config-grpc.config_grpc_provider
}
