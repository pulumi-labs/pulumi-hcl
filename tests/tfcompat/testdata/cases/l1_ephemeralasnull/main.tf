variable "session_token" {
  type      = string
  default   = "ephemeral-default"
  ephemeral = true
}

locals {
  mixed = {
    open  = "visible"
    token = var.session_token
  }
}

output "result" {
  value = ephemeralasnull(local.mixed)
}

output "scalar" {
  value = ephemeralasnull(var.session_token)
}

variable "plain" {
  type    = string
  default = "not-ephemeral"
}

output "passthrough" {
  value = ephemeralasnull(var.plain)
}

output "passthrough_object" {
  value = ephemeralasnull({
    name  = var.plain
    count = 2
  })
}
