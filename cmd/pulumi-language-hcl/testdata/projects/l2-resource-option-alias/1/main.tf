terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "parent" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "aliasURN" {
  pulumi {
    parent  = simple_resource.parent
    aliases = ["urn:pulumi:test::l2-resource-option-alias::simple:index:Resource::aliasURN"]
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "aliasNewName" {
  pulumi {
    aliases = [{
      name = "aliasName"
    }]
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "aliasNoParent" {
  pulumi {
    parent = simple_resource.parent
    aliases = [{
      no_parent = true
    }]
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "aliasParent" {
  pulumi {
    parent = simple_resource.parent
    aliases = [{
      parent_urn = simple_resource.aliasURN.urn
    }]
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "aliasType" {
  pulumi {
    aliases = [{
      type = "component:index:Custom"
    }]
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
