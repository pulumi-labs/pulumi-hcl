# The referenced state stores two set-typed outputs with different element
# types: a set(number) and a set(string). OpenTofu preserves each output's own
# type, so the number set stays numeric (and renders in sorted numeric order).
data "terraform_remote_state" "rs" {
  backend = "local"
  config = {
    path = "remote.tfstate"
  }
}

# jsonencode makes the element type observable: numbers render unquoted.
output "nums_json" {
  value = jsonencode(data.terraform_remote_state.rs.outputs.nums)
}

output "strs_json" {
  value = jsonencode(data.terraform_remote_state.rs.outputs.strs)
}
