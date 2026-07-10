variable "secret_timeout" {
  type      = string
  sensitive = true
  default   = "10m"
}

resource "timeoutable_resource" "test" {
  input_one = "hello"

  timeouts {
    create = var.secret_timeout
  }
}

output "result" {
  value = timeoutable_resource.test.result
}
