variable "version" {
  type = string
}
pulumi {
  required_version_range = var.version
}
