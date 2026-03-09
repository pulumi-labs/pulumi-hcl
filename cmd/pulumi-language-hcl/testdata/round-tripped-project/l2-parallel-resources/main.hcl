terraform {
  required_providers {
    sync = {
      source  = "pulumi/sync"
      version = "3.0.0-alpha.1.internal+exp.sha.2143768"
    }
  }
}

resource "sync_block" "block-1" {
}
resource "sync_block" "block-2" {
}
resource "sync_block" "block-3" {
}
