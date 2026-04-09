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

resource "res5" "module-format:index_Resource:Resource" {
  text = invoke("module-format:index_concatWorld:concatWorld", {
    value = "bonjour"
  }).result
}

resource "res6" "module-format:index_Resource:Resource" {
  text = invoke("module-format:index_concatWorld:concatWorld", {
    value = "youkoso"
  }).result
}

resource "res7" "module-format:index_Resource:Resource" {
  text = invoke("module-format:index_concatWorld:concatWorld", {
    value = "guten tag"
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

output "out5" {
  value = call(res5, "call", {
    input = "x"
  }).output
}

output "out6" {
  value = call(res6, "call", {
    input = "xx"
  }).output
}

output "out7" {
  value = call(res7, "call", {
    input = "xxx"
  }).output
}

