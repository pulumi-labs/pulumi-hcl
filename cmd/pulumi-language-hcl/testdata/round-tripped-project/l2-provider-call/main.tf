terraform {
  required_providers {
    call = {
      source  = "pulumi/call"
      version = "15.7.9"
    }
  }
}

call "defaultRes" "provider_value" {
}

resource "call_custom" "defaultRes" {
  value = "defaultValue"
}
output "defaultProviderValue" {
  value = call.defaultRes.provider_value.result
}
