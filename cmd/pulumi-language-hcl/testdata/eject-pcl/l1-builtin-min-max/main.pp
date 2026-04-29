config "a" "number" {
}

config "b" "number" {
}

config "c" "number" {
}

config "d" "number" {
}

output "maxResult" {
  value = notImplemented("max(var.a, var.b)")
}

output "minResult" {
  value = notImplemented("min(var.a, var.b)")
}

output "intMaxResult" {
  value = notImplemented("max(var.c, var.d)")
}

output "intMinResult" {
  value = notImplemented("min(var.c, var.d)")
}

