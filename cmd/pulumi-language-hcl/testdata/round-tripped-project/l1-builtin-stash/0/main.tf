resource "pulumi_stash" "myStash" {
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
