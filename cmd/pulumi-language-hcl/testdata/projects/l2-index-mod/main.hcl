terraform {
  required_providers {
    index-mod = {
      source  = "pulumi/index-mod"
      version = "35.0.0"
    }
  }
}

data "index-mod_indexmine_concatworld" "invoke_0" {
  value = "hello"
}
data "index-mod_indexmine_nested_concatworld" "invoke_1" {
  value = "goodbye"
}

call "res1" "call" {
  input = "x"
}
call "res2" "call" {
  input = "xx"
}

resource "index-mod_indexmine_resource" "res1" {
  text = data.index-mod_indexmine_concatworld.invoke_0.result
}
resource "index-mod_indexmine_nested_resource" "res2" {
  text = data.index-mod_indexmine_nested_concatworld.invoke_1.result
}
output "out1" {
  value = call.res1.call.output
}
output "out2" {
  value = call.res2.call.output
}
