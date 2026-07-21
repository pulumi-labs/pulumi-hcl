data "pending_lookup" "lookup" {
  name = "widget"
}

output "result" {
  value = data.pending_lookup.lookup.result
}
