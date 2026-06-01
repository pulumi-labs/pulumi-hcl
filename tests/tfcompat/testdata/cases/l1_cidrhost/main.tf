# OpenTofu's `cidrhost` delegates to cidr.HostBig, which counts a negative
# hostnum back from the end of the network's address range. A negative offset
# of -1 is therefore the last host address in the prefix, -2 the second to
# last, and so on. A naive forward-only byte addition would instead ignore the
# sign and return the network address.
output "neg_one" {
  value = cidrhost("10.0.0.0/24", -1)
}

output "neg_two" {
  value = cidrhost("10.0.0.0/24", -2)
}

output "pos" {
  value = cidrhost("10.0.0.0/24", 5)
}
