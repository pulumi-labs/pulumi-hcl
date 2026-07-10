# A provisioner referencing another resource's outputs: OpenTofu treats the
# reference as an implicit dependency and interpolates the created value into
# the command.

resource "simple_resource" "upstream" {
  input_one = "from-upstream"
}

resource "simple_resource" "dependent" {
  input_one = "dependent"

  provisioner "local-exec" {
    command = "echo ${simple_resource.upstream.result}"
  }
}

output "upstream_result" {
  value = simple_resource.upstream.result
}

# A provisioner interpolating a sensitive value: the provisioner still runs
# (with its output suppressed), it does not error.

variable "secret" {
  type      = string
  default   = "hunter2"
  sensitive = true
}

resource "simple_resource" "sensitive_dependent" {
  input_one = "sensitive-dependent"

  provisioner "local-exec" {
    command = "echo ${var.secret}"
  }
}
