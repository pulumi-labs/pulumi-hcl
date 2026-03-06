resource "pulumi_stackreference" "ref" {
  name = "organization/other/dev"
}
output "plain" {
  value = pulumi_stackreference.ref.outputs["plain"]
}
output "secret" {
  value = pulumi_stackreference.ref.outputs["secret"]
}
