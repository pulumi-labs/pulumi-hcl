# Contracting the expansion keeps a["x"] while removing both b and the late
# sibling a["y"]. This exercises the dependencies persisted by stage 0 without
# rerunning the old program during deletion.
resource "order_resource" "a" {
  for_each     = toset(["x"])
  name         = each.key
  delay_delete = true
}

output "b_result" {
  value = "b"
}
