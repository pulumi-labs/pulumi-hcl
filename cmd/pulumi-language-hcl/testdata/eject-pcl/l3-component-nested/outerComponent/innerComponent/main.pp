resource "res" "simple:index:Resource" {
  value = !input
}

config "input" "bool" {
  description = "An input passed to the inner component"
}

output "output" {
  value = res.value
}

