pulumi {
  required_providers {
    replaceonchanges = {
      source  = "pulumi/replaceonchanges"
      version = "25.0.0"
    }
  }
}

// Stage 0: Initial resource creation
// Scenario 1: Schema-based replaceOnChanges on replaceProp
resource "replaceonchanges_resourcea" "schemaReplace" {
  replace_on_changes = ["replaceProp"]
  value              = true
  replace_prop       = true
}
// Scenario 2: Option-based replaceOnChanges on value
resource "replaceonchanges_resourceb" "optionReplace" {
  replace_on_changes = ["value"]
  value              = true
}
// Scenario 3: Both schema and option - will change value
resource "replaceonchanges_resourcea" "bothReplaceValue" {
  replace_on_changes = ["replaceProp", "value"]
  value              = true
  replace_prop       = true
}
// Scenario 4: Both schema and option - will change replaceProp
resource "replaceonchanges_resourcea" "bothReplaceProp" {
  replace_on_changes = ["replaceProp", "value"]
  value              = true
  replace_prop       = true
}
// Scenario 5: No replaceOnChanges - baseline update
resource "replaceonchanges_resourceb" "regularUpdate" {
  value = true
}
// Scenario 6: replaceOnChanges set but no change
resource "replaceonchanges_resourceb" "noChange" {
  replace_on_changes = ["value"]
  value              = true
}
// Scenario 7: replaceOnChanges on value, but only replaceProp changes
resource "replaceonchanges_resourcea" "wrongPropChange" {
  replace_on_changes = ["replaceProp", "value"]
  value              = true
  replace_prop       = true
}
// Scenario 8: Multiple properties in replaceOnChanges array
resource "replaceonchanges_resourcea" "multiplePropReplace" {
  replace_on_changes = ["replaceProp", "value"]
  value              = true
  replace_prop       = true
}
