terraform {
  required_providers {
    replaceonchanges = {
      source  = "pulumi/replaceonchanges"
      version = "25.0.0"
    }
  }
}

// Stage 1: Change properties to trigger replacements
// Scenario 1: Change replaceProp → REPLACE (schema triggers)
resource "replaceonchanges_resourcea" "schemaReplace" {
  replace_on_changes = ["replaceProp"]
  value              = true
  replace_prop       = false // Changed from true
}
// Scenario 2: Change value → REPLACE (option triggers)
resource "replaceonchanges_resourceb" "optionReplace" {
  replace_on_changes = ["value"]
  value              = false // Changed from true
}
// Scenario 3: Change value → REPLACE (option on value triggers)
resource "replaceonchanges_resourcea" "bothReplaceValue" {
  replace_on_changes = ["replaceProp", "value"]
  value              = false // Changed from true
  replace_prop       = true // Unchanged
}
// Scenario 4: Change replaceProp → REPLACE (schema on replaceProp triggers)
resource "replaceonchanges_resourcea" "bothReplaceProp" {
  replace_on_changes = ["replaceProp", "value"]
  value              = true // Unchanged
  replace_prop       = false // Changed from true
}
// Scenario 5: Change value → UPDATE (no replaceOnChanges)
resource "replaceonchanges_resourceb" "regularUpdate" {
  value = false // Changed from true
}
// Scenario 6: No change → SAME (no operation)
resource "replaceonchanges_resourceb" "noChange" {
  replace_on_changes = ["value"]
  value              = true // Unchanged
}
// Scenario 7: Change replaceProp (not value) → UPDATE (marked property unchanged)
resource "replaceonchanges_resourcea" "wrongPropChange" {
  replace_on_changes = ["replaceProp", "value"]
  value              = true // Unchanged (this is marked for replacement)
  replace_prop       = false // Changed from true (this is NOT marked for replacement by option)
}
// Scenario 8: Change value → REPLACE (multiple properties marked)
resource "replaceonchanges_resourcea" "multiplePropReplace" {
  replace_on_changes = ["replaceProp", "value"]
  value              = false // Changed from true
  replace_prop       = true // Unchanged
}
