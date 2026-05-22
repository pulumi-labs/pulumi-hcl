terraform {
  required_providers {
    names = {
      source  = "pulumi/names"
      version = "6.0.0"
    }
  }
}

resource "names_resmap" "res1" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "names_resarray" "res2" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "names_reslist" "res3" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "names_resresource" "res4" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "names_mod_res" "res5" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "names_mod_nested_res" "res6" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
