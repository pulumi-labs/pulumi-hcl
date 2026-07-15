# OpenTofu's local-exec `environment` is a map(string): non-string values are
# coerced (number 5 -> "5", bool true -> "true"). The command below asserts the
# coerced values are present in the child process environment.

resource "simple_resource" "target" {
  input_one = "a"

  provisioner "local-exec" {
    environment = {
      COUNT = 5
      FLAG  = true
    }
    command = "test \"$COUNT\" = \"5\" && test \"$FLAG\" = \"true\""
  }
}
