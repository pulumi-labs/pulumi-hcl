terraform {
  required_providers {
    tls-self-signed-cert = {
      source = "pulumi/tls-self-signed-cert"
    }
  }
}

resource "tls-self-signed-cert_self_signed_certificate" "cert" {
  dns_name                    = "example.com"
  validity_period_hours       = 24
  local_validity_period_hours = 12
  subject = {
    common_name  = "example.com"
    organization = "Pulumi"
  }
}

output "pem" {
  value = tls-self-signed-cert_self_signed_certificate.cert.pem
}
