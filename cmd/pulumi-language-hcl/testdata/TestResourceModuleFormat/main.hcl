pulumi {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_mod_thing" "ubuntu" {
  object_blocks {
    value = true
  }
  object_blocks {
    value = false
  }
}
