terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_mod_thing" "ubuntu" {
  lifecycle {
    create_before_destroy = true
  }
  object_blocks {
    value = true
  }
  object_blocks {
    value = false
  }
}
