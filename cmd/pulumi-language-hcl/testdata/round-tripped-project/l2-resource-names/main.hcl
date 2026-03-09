terraform {
  required_providers {
    names = {
      source  = "pulumi/names"
      version = "6.0.0"
    }
  }
}

resource "names_resmap" "res1" {
  value = true
}
resource "names_resarray" "res2" {
  value = true
}
resource "names_reslist" "res3" {
  value = true
}
resource "names_resresource" "res4" {
  value = true
}
resource "names_mod_res" "res5" {
  value = true
}
resource "names_mod_nested_res" "res6" {
  value = true
}
