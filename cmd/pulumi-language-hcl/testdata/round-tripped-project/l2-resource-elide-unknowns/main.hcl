terraform {
  required_providers {
    output = {
      source  = "pulumi/output"
      version = "23.0.0"
    }
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "pulumi_providers_output" "prov" {
  elide_unknowns = true
}
resource "output_resource" "unknown" {
  provider = pulumi_providers_output.prov
  value    = 1
}
resource "output_complexresource" "complex" {
  provider = pulumi_providers_output.prov
  value    = 1
}
resource "simple_resource" "res" {
  value = output_resource.unknown.output == "hello"
}
resource "simple_resource" "resArray" {
  value = output_complexresource.complex.output_array[0] == "hello"
}
resource "simple_resource" "resMap" {
  value = output_complexresource.complex.output_map["x"] == "hello"
}
resource "simple_resource" "resObject" {
  value = output_complexresource.complex.output_object.output == "hello"
}
output "out" {
  value = output_resource.unknown.output
}
output "outArray" {
  value = output_complexresource.complex.output_array[0]
}
output "outMap" {
  value = output_complexresource.complex.output_map["x"]
}
output "outObject" {
  value = output_complexresource.complex.output_object.output
}
