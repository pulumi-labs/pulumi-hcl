resource "res1" "module-format:mod_Resource:Resource" {
  text = invoke("module-format:mod_concatWorld:concatWorld", {
    value = "hello"
  }).result
}

resource "res2" "module-format:mod_Resource:Resource" {
  text = invoke("module-format:mod_concatWorld:concatWorld", {
    value = "goodbye"
  }).result
}

resource "res3" "module-format:mod/nested_Resource:Resource" {
  text = invoke("module-format:mod/nested_concatWorld:concatWorld", {
    value = "hello"
  }).result
}

resource "res4" "module-format:mod/nested_Resource:Resource" {
  text = invoke("module-format:mod/nested_concatWorld:concatWorld", {
    value = "goodbye"
  }).result
}

output "out1" {
  value = call(res1, "call", {
    input = "x"
  }).output
}

output "out2" {
  value = call(res2, "call", {
    input = "xx"
  }).output
}

output "out3" {
  value = call(res3, "call", {
    input = "x"
  }).output
}

output "out4" {
  value = call(res4, "call", {
    input = "xx"
  }).output
}

