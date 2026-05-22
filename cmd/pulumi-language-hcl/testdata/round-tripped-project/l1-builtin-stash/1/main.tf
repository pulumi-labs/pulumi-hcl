resource "pulumi_stash" "myStash" {
  lifecycle {
    create_before_destroy = true
  }
  input = "ignored"
}
output "stashInput" {
  value = pulumi_stash.myStash.input
}
output "stashOutput" {
  value = pulumi_stash.myStash.output
}
