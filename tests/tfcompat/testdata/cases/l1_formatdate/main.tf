# Terraform's `formatdate` uses lowercase `h`/`hh` for the 24-hour clock and
# uppercase `H`/`HH` for the 12-hour clock, and supports the `Z`/`ZZZZ`/`ZZZZZ`
# timezone tokens and single-quoted literals.
locals {
  t = "2018-01-02T13:05:07Z"
}

output "hh_24" {
  value = formatdate("hh", local.t)
}

output "h_24" {
  value = formatdate("h", local.t)
}

output "cap_hh_12" {
  value = formatdate("HH", local.t)
}

output "cap_h_12" {
  value = formatdate("H", local.t)
}

output "tz_long" {
  value = formatdate("ZZZZZ", local.t)
}

output "tz_mid" {
  value = formatdate("ZZZZ", local.t)
}

output "tz_short" {
  value = formatdate("Z", local.t)
}

output "human" {
  value = formatdate("EEE, DD MMM YYYY hh:mm:ss", local.t)
}

output "literal" {
  value = formatdate("DD 'of' MMMM", local.t)
}
