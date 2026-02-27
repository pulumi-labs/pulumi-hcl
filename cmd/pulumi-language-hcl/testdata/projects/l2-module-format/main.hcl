terraform {
  required_providers {
    module-format = {
      source  = "pulumi/module-format"
      version = "29.0.0"
    }
  }
}

data "module-format_mod_concatworld" "invoke_0" {
  value = "hello"
}
data "module-format_mod_concatworld" "invoke_1" {
  value = "goodbye"
}
data "module-format_mod_nested_concatworld" "invoke_2" {
  value = "hello"
}
data "module-format_mod_nested_concatworld" "invoke_3" {
  value = "goodbye"
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

resource "module-format_mod_resource" "res1" {
  text = data.module-format_mod_concatworld.invoke_0.result
}
resource "module-format_mod_resource" "res2" {
  text = data.module-format_mod_concatworld.invoke_1.result
}
resource "module-format_mod_nested_resource" "res3" {
  text = data.module-format_mod_nested_concatworld.invoke_2.result
}
resource "module-format_mod_nested_resource" "res4" {
  text = data.module-format_mod_nested_concatworld.invoke_3.result
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
