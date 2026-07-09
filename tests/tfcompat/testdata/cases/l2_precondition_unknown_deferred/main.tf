resource "simple_resource" "upstream" {
  input_one = "a"
}

resource "simple_resource" "dependent" {
  input_one = "b"

  lifecycle {
    precondition {
      condition     = simple_resource.upstream.result == "a-false"
      error_message = "upstream result must match"
    }
  }
}
