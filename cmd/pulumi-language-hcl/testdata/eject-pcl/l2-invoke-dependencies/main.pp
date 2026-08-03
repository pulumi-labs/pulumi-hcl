resource "first" "simple:index:Resource" {
  value = false
}

// assert that resource second depends on resource first
// because it uses .secret from the invoke which depends on first
resource "second" "simple:index:Resource" {
  value = invoke("simple-invoke:index:secretInvoke", {
    value          = "hello"
    secretResponse = first.value
  }).secret
}

resource "third" "simple-invoke:index:StringResource" {
  text = "third"
}

resource "fourth" "simple-invoke:index:StringResource" {
  text = invoke("simple-invoke:index:getText", {
    text = third.text
  }).result
}

