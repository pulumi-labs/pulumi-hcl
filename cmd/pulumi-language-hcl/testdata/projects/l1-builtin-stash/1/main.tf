resource "pulumi_stash" "myStash" {
  input = "ignored"
}
output "stashInput" {
  value = pulumi_stash.myStash.input
}
output "stashOutput" {
  value = pulumi_stash.myStash.output
}
