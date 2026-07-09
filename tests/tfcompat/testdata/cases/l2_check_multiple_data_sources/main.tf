check "two_data" {
  data "simple_lookup" "a" {
    query = "a"
  }

  data "simple_lookup" "b" {
    query = "b"
  }

  assert {
    condition     = data.simple_lookup.a.prefix_result == "-a"
    error_message = "mismatch"
  }
}
