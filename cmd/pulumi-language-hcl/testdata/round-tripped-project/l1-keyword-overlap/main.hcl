locals {
  class = "class_output_string"
}
locals {
  export = "export_output_string"
}
locals {
  import = "import_output_string"
}
locals {
  mod = "mod_output_string"
}
locals {
  object = {
    "object" = "object_output_string"
  }
}
locals {
  self = "self_output_string"
}
locals {
  this = "this_output_string"
}
locals {
  if = "if_output_string"
}
output "class" {
  value = local.class
}
output "export" {
  value = local.export
}
output "import" {
  value = local.import
}
output "mod" {
  value = local.mod
}
output "object" {
  value = local.object
}
output "self" {
  value = local.self
}
output "this" {
  value = local.this
}
output "if" {
  value = local.if
}
