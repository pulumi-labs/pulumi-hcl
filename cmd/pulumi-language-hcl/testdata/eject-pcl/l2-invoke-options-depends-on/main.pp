resource "explicitProvider" "pulumi:providers:simple-invoke" {
}

resource "first" "simple-invoke:index:StringResource" {
  text = "first hello"
}

resource "second" "simple-invoke:index:StringResource" {
  text = invoke("simple-invoke:index:myInvoke", {
    value = "hello"
    }, {
    dependsOn = [first]
  }).result
}

output "hello" {
  value = invoke("simple-invoke:index:myInvoke", {
    value = "hello"
    }, {
    dependsOn = [first]
  }).result
}

