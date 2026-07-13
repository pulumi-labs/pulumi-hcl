variable "name" {
  type = string
}

resource "simple_resource" "r" {
  input_one = var.name
  input_two = false

  lifecycle {
    postcondition {
      condition     = self.result == "${var.name}-false"
      error_message = "unexpected result"
    }
  }
}

output "result" {
  value = simple_resource.r.result
}
