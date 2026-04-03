terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "parent" {
  value = true
}
resource "simple_resource" "aliasURN" {
  parent  = simple_resource.parent
  aliases = ["urn:pulumi:test::l2-resource-option-alias::simple:index:Resource::aliasURN"]
  value   = true
}
resource "simple_resource" "aliasNewName" {
  aliases = [{
    name = "aliasName"
  }]
  value = true
}
resource "simple_resource" "aliasNoParent" {
  parent = simple_resource.parent
  aliases = [{
    no_parent = true
  }]
  value = true
}
resource "simple_resource" "aliasParent" {
  parent = simple_resource.parent
  aliases = [{
    parent_urn = simple_resource.aliasURN.urn
  }]
  value = true
}
resource "simple_resource" "aliasType" {
  aliases = [{
    type = "component:index:Custom"
  }]
  value = true
}
