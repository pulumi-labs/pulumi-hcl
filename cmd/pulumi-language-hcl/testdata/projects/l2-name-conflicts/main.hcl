pulumi {
  required_providers {
    module-format = {
      source  = "pulumi/module-format"
      version = "29.0.0"
    }
    names = {
      source  = "pulumi/names"
      version = "6.0.0"
    }
  }
}

resource "names_mod_res" "namesResource" {
  value = var.names
}
resource "module-format_mod_resource" "modResource" {
  text ="${var.mod}-${var.Mod}"
}
variable "names" {
  type    = bool
  default = true
}
variable "Names" {
  type    = bool
  default = true
}
variable "mod" {
  type    = string
  default = "module"
}
variable "Mod" {
  type    = string
  default = "format"
}
output "namesResourceVal" {
  value = names_mod_res.namesResource.value
}
output "modResourceText" {
  value = module-format_mod_resource.modResource.text
}
output "nameVariables" {
  value = var.names && var.Names
}
output "modVariables" {
  value ="${var.mod}-${var.Mod}"
}
