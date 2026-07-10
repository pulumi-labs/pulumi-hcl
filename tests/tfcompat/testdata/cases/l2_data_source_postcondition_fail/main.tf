data "simple_lookup" "d" {
  query = "hello"

  lifecycle {
    postcondition {
      condition     = self.prefix_result == "wrong-expected-value"
      error_message = "DATA_POSTCONDITION_VIOLATED"
    }
  }
}

output "looked_up" {
  value = data.simple_lookup.d.prefix_result
}
