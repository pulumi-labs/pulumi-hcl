variable "expected" {
  type = string
}

resource "simple_resource" "guarded" {
  input_one = "ok"

  lifecycle {
    precondition {
      condition     = var.expected == "ok"
      error_message = "should never fire"
    }
  }
}

output "id" {
  value = simple_resource.guarded.id
}
