resource "schemaReplace" "replaceonchanges:index:ResourceA" {
  value       = true
  replaceProp = false
  options {
    replaceOnChanges = [replaceProp]
  }
}

resource "optionReplace" "replaceonchanges:index:ResourceB" {
  value = false
  options {
    replaceOnChanges = [value]
  }
}

resource "bothReplaceValue" "replaceonchanges:index:ResourceA" {
  value       = false
  replaceProp = true
  options {
    replaceOnChanges = [replaceProp, value]
  }
}

resource "bothReplaceProp" "replaceonchanges:index:ResourceA" {
  value       = true
  replaceProp = false
  options {
    replaceOnChanges = [replaceProp, value]
  }
}

resource "regularUpdate" "replaceonchanges:index:ResourceB" {
  value = false
}

resource "noChange" "replaceonchanges:index:ResourceB" {
  value = true
  options {
    replaceOnChanges = [value]
  }
}

resource "wrongPropChange" "replaceonchanges:index:ResourceA" {
  value       = true
  replaceProp = false
  options {
    replaceOnChanges = [replaceProp, value]
  }
}

resource "multiplePropReplace" "replaceonchanges:index:ResourceA" {
  value       = false
  replaceProp = true
  options {
    replaceOnChanges = [replaceProp, value]
  }
}

