terraform {
  required_providers {
    call = {
      source  = "pulumi/call"
      version = "15.7.9"
    }
  }
}

call "explicitRes" "provider_value" {
}
call "explicitProv" "identity" {
}
call "explicitProv" "prefixed" {
  prefix = "call-prefix-"
}

resource "pulumi_providers_call" "explicitProv" {
  value = "explicitProvValue"
}
resource "call_custom" "explicitRes" {
  provider = pulumi_providers_call.explicitProv
  value    = "explicitValue"
}
output "explicitProviderValue" {
  value = call.explicitRes.provider_value.result
}
output "explicitProvFromIdentity" {
  value = call.explicitProv.identity.result
}
output "explicitProvFromPrefixed" {
  value = call.explicitProv.prefixed.result
}
