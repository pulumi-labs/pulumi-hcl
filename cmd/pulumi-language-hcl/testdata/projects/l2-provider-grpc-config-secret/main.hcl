terraform {
  required_providers {
    config-grpc = {
      source  = "pulumi/config-grpc"
      version = "1.0.0"
    }
  }
}

data "config-grpc_tosecret" "invoke_0" {
  string1 = "SECRET"
}
data "config-grpc_tosecret" "invoke_1" {
  int1 = 1234567890
}
data "config-grpc_tosecret" "invoke_2" {
  num1 = 123456.789
}
data "config-grpc_tosecret" "invoke_3" {
  bool1 = true
}
data "config-grpc_tosecret" "invoke_4" {
  list_string1 = ["SECRET", "SECRET2"]
}
data "config-grpc_tosecret" "invoke_5" {
  string1 = "SECRET"
}
data "config-grpc_tosecret" "invoke_6" {
  string1 = "SECRET"
}
data "config-grpc_tosecret" "invoke_7" {
  string1 = "SECRET"
}

resource "pulumi_providers_config-grpc" "config_grpc_provider" {
  string1      = data.config-grpc_tosecret.invoke_0.string1
  int1         = data.config-grpc_tosecret.invoke_1.int1
  num1         = data.config-grpc_tosecret.invoke_2.num1
  bool1        = data.config-grpc_tosecret.invoke_3.bool1
  list_string1 = data.config-grpc_tosecret.invoke_4.list_string1
  list_string2 = ["VALUE", data.config-grpc_tosecret.invoke_5.string1]
  map_string2 = {
    "key1" = "value1"
    "key2" = data.config-grpc_tosecret.invoke_6.string1
  }
  obj_string2 = {
    x = data.config-grpc_tosecret.invoke_7.string1
  }
}
resource "config-grpc_configfetcher" "config" {
  provider = pulumi_providers_config-grpc.config_grpc_provider
}
