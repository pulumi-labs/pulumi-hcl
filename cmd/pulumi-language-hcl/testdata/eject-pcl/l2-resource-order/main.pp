resource "res2" "simple:index:Resource" {
  value = localVar
}

resource "res1" "simple:index:Resource" {
  value = true
}

// This test asserts that PCL declaration order does not need to match usage order. That is a resource can be declared
// lower in the file than it is first referenced.
output "out" {
  value = res2.value
}

localVar = res1.value

