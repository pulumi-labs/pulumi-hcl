variable "session_token" {
  type      = string
  default   = "ephemeral-default"
  ephemeral = true
}

output "result" {
  value = "ok"
}
