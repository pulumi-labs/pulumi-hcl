// Test that "pkg:typ" type tokens are accepted in PCL and are correctly expanded out. We also have an L2 test around
// this but it's worth checking with the pulumi schema as it would be too easy for codegen to special case it differently.
resource "pulumi_stash" "myStash" {
  lifecycle {
    create_before_destroy = true
  }
  input = "test"
}
output "stashInput" {
  value = pulumi_stash.myStash.input
}
output "stashOutput" {
  value = pulumi_stash.myStash.output
}
