# A precondition, postcondition, and create/destroy provisioners on a builtin
# terraform_data resource (no external providers). Each provisioner drops a
# marker file so the test can observe that it ran.
variable "marker" {
  type = string
}

resource "terraform_data" "guarded" {
  input = var.marker

  lifecycle {
    precondition {
      condition     = var.marker != ""
      error_message = "PRECONDITION_FAILED"
    }
    postcondition {
      condition     = self.input != ""
      error_message = "POSTCONDITION_FAILED"
    }
  }

  provisioner "local-exec" {
    command = "touch '${var.marker}/created'"
  }

  provisioner "local-exec" {
    when    = destroy
    command = "touch '${self.input}/destroyed'"
  }
}

output "marker" {
  value = terraform_data.guarded.input
}
