terraform {
  required_providers {
    sync = {
      source  = "pulumi/sync"
      version = "3.0.0-alpha.1.internal+exp.sha.2143768"
    }
  }
}

resource "sync_block" "block-1" {
  lifecycle {
    create_before_destroy = true
  }
}
resource "sync_block" "block-2" {
  lifecycle {
    create_before_destroy = true
  }
}
resource "sync_block" "block-3" {
  lifecycle {
    create_before_destroy = true
  }
}
