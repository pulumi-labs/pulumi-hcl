pulumi {
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

// This test checks that when a provider doesn't return properties for fields it considers unknown the runtime
// can still access that field as an output.
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
// Try and use the unknown output as an input to another resource to check that it doesn't cause any issues.
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
// And try to use it has an output
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
