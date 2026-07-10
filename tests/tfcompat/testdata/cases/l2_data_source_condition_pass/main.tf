variable "expected_suffix" {
  type    = string
  default = "hello"
}

data "simple_lookup" "d" {
  query = "hello"

  lifecycle {
    precondition {
      condition     = length(var.expected_suffix) > 0
      error_message = "unreachable precondition"
    }
    postcondition {
      condition     = endswith(self.prefix_result, var.expected_suffix)
      error_message = "unreachable postcondition"
    }
  }
}

output "looked_up" {
  value = data.simple_lookup.d.prefix_result
}
