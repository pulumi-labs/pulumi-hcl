# `d` and `r` are completely independent, but OpenTofu reads data sources
# whose config is fully known during plan, and apply starts only after plan
# completes: the delayed read still records before the create, [read d,
# create r]. A runtime that schedules the two concurrently lets the undelayed
# create record ahead of the delayed read — a deterministic order flip.
data "order_data" "d" {
  name       = "d"
  delay_read = true
}

resource "order_resource" "r" {
  name = "r"
}

output "d_result" {
  value = data.order_data.d.result
}
