resource "aA-Alpha_alpha____" "simple:index:Resource" {
  __logicalName = "aA-Alpha_alpha.🤯⁉️"
  value         = cC-Charlie_charlie____
}

config "cC-Charlie_charlie____" "bool" {
  __logicalName = "cC-Charlie_charlie.😃⁉️"
}

output "bB-Beta_beta.💜⁉" {
  __logicalName = "bB-Beta_beta.💜⁉"
  value         = aA-Alpha_alpha____.value
}

// New format for output logical name because outputs don't have separate logical names. Even nodejs which just
// does "export" normally for outputs needs that export _to be_ the output name and so if the "logical name"
// isn't a valid nodejs export we have to output it differently.
output "dD-Delta_delta.🔥⁉" {
  __logicalName = "dD-Delta_delta.🔥⁉"
  value         = aA-Alpha_alpha____.value
}

