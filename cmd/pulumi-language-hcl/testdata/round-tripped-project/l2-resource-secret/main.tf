terraform {
  required_providers {
    secret = {
      source  = "pulumi/secret"
      version = "14.0.0"
    }
  }
}

resource "secret_resource" "res" {
  lifecycle {
    create_before_destroy = true
  }
  private = "closed"
  public  = "open"
  private_data = {
    private = "closed"
    public  = "open"
  }
  public_data = {
    private = "closed"
    public  = "open"
  }
}
