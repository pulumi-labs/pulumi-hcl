terraform {
  required_providers {
    module-format = {
      source  = "pulumi/module-format"
      version = "29.0.0"
    }
  }
}

data "module-format_mod_concat_world" "invoke_0" {
  value = "hello"
}
data "module-format_mod_concat_world" "invoke_1" {
  value = "goodbye"
}
data "module-format_mod_nested_concat_world" "invoke_2" {
  value = "hello"
}
data "module-format_mod_nested_concat_world" "invoke_3" {
  value = "goodbye"
}
data "module-format_concat_world" "invoke_4" {
  value = "bonjour"
}
data "module-format_concat_world" "invoke_5" {
  value = "youkoso"
}
data "module-format_concat_world" "invoke_6" {
  value = "guten tag"
}

call "res1" "call" {
  input = "x"
}
call "res2" "call" {
  input = "xx"
}
call "res3" "call" {
  input = "x"
}
call "res4" "call" {
  input = "xx"
}
call "res5" "call" {
  input = "x"
}
call "res6" "call" {
  input = "xx"
}
call "res7" "call" {
  input = "xxx"
}

// This tests that PCL allows both fully specified type tokens, and tokens that only specify the module and
// member name.
// First use the fully specified token to invoke and create a resource.
resource "module-format_mod_resource" "res1" {
  text = data.module-format_mod_concat_world.invoke_0.result
}
// Next use just the module name as defined by the module format
resource "module-format_mod_resource" "res2" {
  text = data.module-format_mod_concat_world.invoke_1.result
}
// First use the fully specified token to invoke and create a resource.
resource "module-format_mod_nested_resource" "res3" {
  text = data.module-format_mod_nested_concat_world.invoke_2.result
}
// Next use just the module name as defined by the module format
resource "module-format_mod_nested_resource" "res4" {
  text = data.module-format_mod_nested_concat_world.invoke_3.result
}
// First use the fully specified token to invoke and create a resource in the index module.
resource "module-format_resource" "res5" {
  text = data.module-format_concat_world.invoke_4.result
}
// Next use just the module name as defined by the module format
resource "module-format_resource" "res6" {
  text = data.module-format_concat_world.invoke_5.result
}
// Next use the short, 2 component, form because this is the index module
resource "module-format_resource" "res7" {
  text = data.module-format_concat_world.invoke_6.result
}
output "out1" {
  value = call.res1.call.output
}
output "out2" {
  value = call.res2.call.output
}
output "out3" {
  value = call.res3.call.output
}
output "out4" {
  value = call.res4.call.output
}
output "out5" {
  value = call.res5.call.output
}
output "out6" {
  value = call.res6.call.output
}
output "out7" {
  value = call.res7.call.output
}
