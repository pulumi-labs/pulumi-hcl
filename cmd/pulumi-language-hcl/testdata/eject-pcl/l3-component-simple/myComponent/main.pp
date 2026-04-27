resource "res" "simple:index:Resource" {
  value = input
}

config "input" "bool" {
  description = "A simple input"
}

output "output" {
  value = res.value
}

