terraform {
  required_providers {
    component = {
      source  = "pulumi/component"
      version = "13.3.7"
    }
  }
}

call "component1" "identity" {
}
call "component1" "prefixed" {
  prefix = "foo-"
}

resource "component_componentcallable" "component1" {
  value = "bar"
}
output "from_identity" {
  value = call.component1.identity.result
}
output "from_prefixed" {
  value = call.component1.prefixed.result
}
