resource "namesResource" "names:mod:Res" {
  value = names
}

resource "modResource" "module-format:mod_Resource:Resource" {
  text = "${mod}-${Mod}"
}

config "names" "bool" {
  default = true
}

config "Names" "bool" {
  default = true
}

config "mod" "string" {
  default = "module"
}

config "Mod" "string" {
  default = "format"
}

output "namesResourceVal" {
  value = namesResource.value
}

output "modResourceText" {
  value = modResource.text
}

output "nameVariables" {
  value = names && Names
}

output "modVariables" {
  value = "${mod}-${Mod}"
}

