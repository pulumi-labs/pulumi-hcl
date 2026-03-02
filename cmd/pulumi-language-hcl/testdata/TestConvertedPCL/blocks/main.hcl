terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_server" "myServer" {
  name = "my-server"
  network_rules {
    protocol = "tcp"
    port     = 443
  }
  network_rules {
    protocol = "udp"
    port     = 53
  }
}
