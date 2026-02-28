terraform {
  required_providers {
    output = {
      source  = "pulumi/output"
      version = "23.0.0"
    }
  }
}

resource "output_complexresource" "res" {
  value = 1
}
output "entriesOutput" {
  value = entries(output_complexresource.res.output_map)
}
output "lookupOutput" {
  value = lookup(output_complexresource.res.output_map, "x", "default")
}
output "lookupOutputDefault" {
  value = lookup(output_complexresource.res.output_map, "y", "default")
}
output "entriesObjectOutput" {
  value = entries(output_complexresource.res.output_object)
}
output "lookupObjectOutput" {
  value = lookup(output_complexresource.res.output_object, "output", "default")
}
output "lookupObjectOutputDefault" {
  value = lookup(output_complexresource.res.output_object, "missing", "default")
}
