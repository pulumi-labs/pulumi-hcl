resource "myStash" "pulumi:index:Stash" {
  input = "test"
}

output "stashInput" {
  value = myStash.input
}

output "stashOutput" {
  value = myStash.output
}

