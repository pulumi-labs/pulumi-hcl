terraform {
  required_providers {
    secret = {
      source  = "pulumi/secret"
      version = "14.0.0"
    }
  }
}

resource "secret_resource" "res" {
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
