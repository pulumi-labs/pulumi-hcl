resource "pulumi_stash" "myStash" {
  input = "test"
}
output "stashInput" {
  value = pulumi_stash.myStash.input
}
output "stashOutput" {
  value = pulumi_stash.myStash.output
}
