terraform {
  required_providers {
    union = {
      source = "union"
    }
  }
}

variable "disc" {
  type      = string
  sensitive = true
  default   = "a"
}

resource "union_thing" "t" {
  cfg = {
    type  = var.disc
    value = "x"
  }
}
