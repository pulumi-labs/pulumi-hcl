# The referenced state stores an output whose value is null (typed string).
# OpenTofu preserves null outputs: referencing the outputs object keeps the
# `nullstr` key with a null value, so jsonencode renders it as `"nullstr":null`.
data "terraform_remote_state" "rs" {
  backend = "local"
  config = {
    path = "remote.tfstate"
  }
}

# jsonencode the whole outputs object so the presence of the null-valued output
# is observable without referencing (and erroring on) a possibly-absent key.
output "outputs_json" {
  value = jsonencode(data.terraform_remote_state.rs.outputs)
}
