resource "res2" "simple:index:Resource" {
  value = localVar
}

resource "res1" "simple:index:Resource" {
  value = true
}

output "out" {
  value = res2.value
}

localVar = res1.value

