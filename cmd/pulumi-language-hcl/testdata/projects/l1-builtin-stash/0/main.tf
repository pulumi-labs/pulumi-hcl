resource "pulumi_stash" "myStash" {
  lifecycle {
    create_before_destroy = true
  }
  input = {
    "key" = ["value", "s"]
    ""    = false
  }
}
output "stashInput" {
  value = pulumi_stash.myStash.input
}
output "stashOutput" {
  value = pulumi_stash.myStash.output
}
