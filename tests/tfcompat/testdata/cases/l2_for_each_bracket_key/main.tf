# `for_each` keys are opaque strings in OpenTofu, so keys that contain a
# `[` character followed by non-numeric text still address distinct
# instances (`web["group[a"]` / `web["group[b"]`) and apply cleanly.
resource "simple_resource" "web" {
  for_each  = toset(["group[a", "group[b"])
  input_one = each.key
  input_two = false
}

output "web" {
  value = { for k, v in simple_resource.web : k => v.result }
}
