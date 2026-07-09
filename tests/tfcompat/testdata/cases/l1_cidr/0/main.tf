output "host_pos" {
  value = cidrhost("10.0.0.0/24", 5)
}

output "host_neg_one" {
  value = cidrhost("10.0.0.0/24", -1)
}

output "host_neg_two" {
  value = cidrhost("10.0.0.0/24", -2)
}

output "host_leading_zeros" {
  value = cidrhost("010.001.0.0/24", 5)
}

output "netmask" {
  value = cidrnetmask("10.0.0.0/8")
}

output "netmask_leading_zeros" {
  value = cidrnetmask("010.001.0.0/24")
}

output "subnet_leading_zeros" {
  value = cidrsubnet("010.001.0.0/16", 8, 2)
}

output "subnets_leading_zeros" {
  value = cidrsubnets("010.001.0.0/16", 8, 8)
}
