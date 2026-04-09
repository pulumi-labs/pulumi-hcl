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
data "module-format_concatworld" "invoke_4" {
  value = "bonjour"
}
data "module-format_concatworld" "invoke_5" {
  value = "youkoso"
}
data "module-format_concatworld" "invoke_6" {
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
resource "module-format_resource" "res5" {
  text = data.module-format_concatworld.invoke_4.result
}
resource "module-format_resource" "res6" {
  text = data.module-format_concatworld.invoke_5.result
}
resource "module-format_resource" "res7" {
  text = data.module-format_concatworld.invoke_6.result
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
