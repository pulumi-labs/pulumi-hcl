terraform {
  required_providers {
    names = {
      source  = "pulumi/names"
      version = "6.0.0"
    }
  }
}

resource "names_res_map" "res1" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "names_res_array" "res2" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "names_res_list" "res3" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "names_res_resource" "res4" {
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
