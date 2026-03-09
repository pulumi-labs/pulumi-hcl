resource "res" "simple:index:Resource" {
  value = input
}

config "input" "bool" {
}

output "output" {
  value = res.value
}

