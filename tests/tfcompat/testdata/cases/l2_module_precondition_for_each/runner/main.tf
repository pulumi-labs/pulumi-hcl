variable "name" {
  type = string
}

resource "simple_resource" "r" {
  input_one = var.name
  input_two = false

  lifecycle {
    precondition {
      condition     = var.name != ""
      error_message = "name must be set"
    }
  }
}

output "result" {
  value = simple_resource.r.result
}
