variable "name" {
  type = string
}

check "name_check" {
  assert {
    condition     = var.name == "this-will-never-match"
    error_message = "module check assertion did not hold"
  }
}

output "result" {
  value = var.name
}
