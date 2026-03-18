resource "first-untainted" "simple:index:Resource" {
  value = true
}

resource "first-tainted" "simple:index:Resource" {
  value = !input
}

config "input" "bool" {
}

output "untainted" {
  value = first-untainted.value
}

output "tainted" {
  value = first-tainted.value
}

