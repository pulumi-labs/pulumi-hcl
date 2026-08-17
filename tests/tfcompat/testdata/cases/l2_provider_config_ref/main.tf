resource "order_resource" "config" {
  name = "config"
}

provider "order" {
  alias = "configured"
  token = order_resource.config.result
}

resource "order_resource" "consumer" {
  provider = order.configured
  name     = "consumer"
}

output "config_result" {
  value = order_resource.config.result
}
