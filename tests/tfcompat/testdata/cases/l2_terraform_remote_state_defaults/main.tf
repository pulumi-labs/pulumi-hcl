# terraform_remote_state's `defaults` supplies a value for any output the
# referenced state does not define, while a present output wins over its
# default. The fixture state defines only `greeting`, so `number` and `missing`
# come from defaults and `greeting` keeps its state value. pulumi-language-hcl
# overlays `defaults` on the getLocalReference result the same way.
data "terraform_remote_state" "rs" {
  backend = "local"
  config = {
    path = "remote.tfstate"
  }
  defaults = {
    greeting = "default-greeting"
    number   = 99
    missing  = "fallback"
  }
}

output "greeting" {
  value = data.terraform_remote_state.rs.outputs.greeting
}

output "number" {
  value = data.terraform_remote_state.rs.outputs.number
}

output "missing" {
  value = data.terraform_remote_state.rs.outputs.missing
}
