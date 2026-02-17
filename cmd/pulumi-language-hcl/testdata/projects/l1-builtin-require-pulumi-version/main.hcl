variable "version" {
  type = string
}
pulumi {
  requiredVersionRange = var.version
}
