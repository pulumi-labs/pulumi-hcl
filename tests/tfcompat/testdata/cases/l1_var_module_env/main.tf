# `TF_VAR_<name>` sets root variables only. The child module's `who` gets no
# input here, so it takes its own default even though TF_VAR_who is set.
module "child" {
  source = "./child"
}

output "who" {
  value = module.child.who
}
