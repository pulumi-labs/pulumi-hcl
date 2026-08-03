terraform {
  required_providers {
    fail_on_create = {
      source  = "pulumi/fail_on_create"
      version = "4.0.0"
    }
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "fail_on_create_resource" "failing" {
  lifecycle {
    create_before_destroy = true
  }
  value = false
}
resource "simple_resource" "recovered_value" {
  lifecycle {
    create_before_destroy = true
  }
  value = recover(fail_on_create_resource.failing.value, error != "")
}
resource "simple_resource" "independent" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
output "recovered" {
  value = recover(pulumiresourceurn(fail_on_create_resource.failing), "recovered: ${error}")
}
