terraform {
  required_providers {
    replaceonchanges = {
      source  = "pulumi/replaceonchanges"
      version = "25.0.0"
    }
  }
}

resource "replaceonchanges_resourcea" "schemaReplace" {
  replace_on_changes = ["replaceProp"]
  value              = true
  replace_prop       = true
}
resource "replaceonchanges_resourceb" "optionReplace" {
  replace_on_changes = ["value"]
  value              = true
}
resource "replaceonchanges_resourcea" "bothReplaceValue" {
  replace_on_changes = ["replaceProp", "value"]
  value              = true
  replace_prop       = true
}
resource "replaceonchanges_resourcea" "bothReplaceProp" {
  replace_on_changes = ["replaceProp", "value"]
  value              = true
  replace_prop       = true
}
resource "replaceonchanges_resourceb" "regularUpdate" {
  value = true
}
resource "replaceonchanges_resourceb" "noChange" {
  replace_on_changes = ["value"]
  value              = true
}
resource "replaceonchanges_resourcea" "wrongPropChange" {
  replace_on_changes = ["replaceProp", "value"]
  value              = true
  replace_prop       = true
}
resource "replaceonchanges_resourcea" "multiplePropReplace" {
  replace_on_changes = ["replaceProp", "value"]
  value              = true
  replace_prop       = true
}
