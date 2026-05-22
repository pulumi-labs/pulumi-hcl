resource "ref" "pulumi:pulumi:StackReference" {
  name = "organization/other/dev"
}

output "plain" {
  value = ref.outputs["plain"]
}

output "secret" {
  value = ref.outputs["secret"]
}

output "secret_unsecret" {
  value = unsecret(ref.outputs["secret"])
}

