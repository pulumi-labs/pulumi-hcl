resource "pulumi_stackreference" "ref" {
  lifecycle {
    create_before_destroy = true
  }
  name = "organization/other/dev"
}
output "plain" {
  value = pulumi_stackreference.ref.outputs["plain"]
}
output "secret" {
  value = pulumi_stackreference.ref.outputs["secret"]
}
output "secret_unsecret" {
  value = nonsensitive(pulumi_stackreference.ref.outputs["secret"])
}
