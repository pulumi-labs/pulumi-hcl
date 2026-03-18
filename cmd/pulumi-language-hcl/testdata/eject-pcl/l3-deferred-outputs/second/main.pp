resource "second-untainted" "simple:index:Resource" {
  value = true
}

resource "second-tainted" "simple:index:Resource" {
  value = !input
}

config "input" "bool" {
}

output "untainted" {
  value = second-untainted.value
}

output "tainted" {
  value = second-tainted.value
}

