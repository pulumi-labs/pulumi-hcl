pulumi {
  required_providers {
    names = {
      source  = "pulumi/names"
      version = "6.0.0"
    }
  }
}

resource "names_res_map" "res1" {
  value = true
}
resource "names_res_array" "res2" {
  value = true
}
resource "names_res_list" "res3" {
  value = true
}
resource "names_res_resource" "res4" {
  value = true
}
resource "names_mod_res" "res5" {
  value = true
}
resource "names_mod_nested_res" "res6" {
  value = true
}
