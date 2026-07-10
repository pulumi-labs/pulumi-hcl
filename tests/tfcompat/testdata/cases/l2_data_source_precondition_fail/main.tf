variable "trigger_failure" {
  type    = bool
  default = true
}

data "simple_lookup" "d" {
  query = "hello"

  lifecycle {
    precondition {
      condition     = !var.trigger_failure
      error_message = "DATA_PRECONDITION_VIOLATED"
    }
  }
}

output "looked_up" {
  value = data.simple_lookup.d.prefix_result
}
