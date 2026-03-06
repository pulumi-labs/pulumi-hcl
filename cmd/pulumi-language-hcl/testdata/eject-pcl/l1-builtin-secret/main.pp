config "aSecret" "string" {
}

config "notSecret" "string" {
}

output "roundtripSecret" {
  value = aSecret
}

output "roundtripNotSecret" {
  value = notSecret
}

output "double" {
  value = secret(aSecret)
}

output "open" {
  value = unsecret(aSecret)
}

output "close" {
  value = secret(notSecret)
}

