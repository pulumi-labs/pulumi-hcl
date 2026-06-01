resource "pulumi_stack_reference" "ref" {
  lifecycle {
    create_before_destroy = true
  }
  name = "organization/other/dev"
}
output "plain" {
  value = pulumi_stack_reference.ref.outputs["plain"]
}
output "secret" {
  value = pulumi_stack_reference.ref.outputs["secret"]
}
output "secret_unsecret" {
  value = nonsensitive(pulumi_stack_reference.ref.outputs["secret"])
}
