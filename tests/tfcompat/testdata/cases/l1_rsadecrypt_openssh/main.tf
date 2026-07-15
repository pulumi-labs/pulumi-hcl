# `rsadecrypt` decrypts a base64-encoded RSA-PKCS1v15 ciphertext with the
# given PEM private key. OpenTofu parses the key via ssh.ParseRawPrivateKey,
# which accepts the OpenSSH private-key format ("BEGIN OPENSSH PRIVATE KEY")
# in addition to PKCS#1 and PKCS#8. The ciphertext below was produced by
# encrypting "hello-rsadecrypt" with the matching RSA public key.
locals {
  ciphertext = "BQVXU38vaY5d1Unt31rC8i/jN3EgZndLwutYcy0ZXNwJdHE5DGjyw5feoWg5CX23LHirFkNraDslnIbUcudMxSkXr9eBf1GDZwpoAkS+cQ0YSRULFE1l9noiaK9UGpkcMNMu2AyzPUEMzjBlKDlbAYD+/t1SNR1aqvMYfp3MXxBwiy+ylg4ZG1Y/viIrJJP8mQgaNr48IJNfkG4+bMT+z2eXpyfAiWZY1pTbg84GqHXJCUk3qxeCnIBI1skbOk1cGbneIGrTL8xGt2QZ4sap2DjurEPON+7w7XmQ2AvU829ogH9Zs/dlEkZTQacGh+sFR+gu2N4HmYUzOtC462I2wQ=="
  private_key = <<-EOT
  -----BEGIN OPENSSH PRIVATE KEY-----
  b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABFwAAAAdzc2gtcn
  NhAAAAAwEAAQAAAQEAuQ2nEhILwjrgAr/0fNViWHqTfzuBBsjvqG99RR7at2PEnQqv67Bm
  SpYK4n/Gd3T9+/b1NA41QP8yiN+q++aHjS5iQw01s4XxX8b75yPSjwKSQUYRLqB45E0YS4
  1wr/N93URfMArWHluEdiEyX47RbdE6AIRajVFJ70GU2t9bmo1w3j584ahdS/2cr8Db+wRn
  CB+iJyXwN4OCfb15QKH82POs7AhpHWy9+Gd7SCsJLQjg72uP9mrnU2HryHkbOTLRUEsP7+
  OwvGFF9N/2PBQQSu91M2lU0hdogSrcvjmn5MDxd34DtMB8ISxsh2BVfv7Gdc6iUq3z0T3K
  Z0HmgPUx9wAAA9DoGo2U6BqNlAAAAAdzc2gtcnNhAAABAQC5DacSEgvCOuACv/R81WJYep
  N/O4EGyO+ob31FHtq3Y8SdCq/rsGZKlgrif8Z3dP379vU0DjVA/zKI36r75oeNLmJDDTWz
  hfFfxvvnI9KPApJBRhEuoHjkTRhLjXCv833dRF8wCtYeW4R2ITJfjtFt0ToAhFqNUUnvQZ
  Ta31uajXDePnzhqF1L/ZyvwNv7BGcIH6InJfA3g4J9vXlAofzY86zsCGkdbL34Z3tIKwkt
  CODva4/2audTYevIeRs5MtFQSw/v47C8YUX03/Y8FBBK73UzaVTSF2iBKty+OafkwPF3fg
  O0wHwhLGyHYFV+/sZ1zqJSrfPRPcpnQeaA9TH3AAAAAwEAAQAAAQABMFJDbnQ+4ivwOJV0
  e9Zu5RKvfY1dosrPVTAD0qfrB6wKqjfpFrABiKc3P0TiHZFIHhUDKZgz+6+ya2VoytlSEd
  s1vQ78QT8Es32IxZUjsAuKec3Ac+1y4f/m9Fil+LV1R2wpHdi0Rzg5ngr5zCwSPYbW3ALM
  55nG/K/dHBQ1kPI9viqgsDBoff2P3/8HZWNhggiXMFXGZ748jgZZFmibcix4tWLV5+eAvg
  7/WsoK8qNdz9kQ4rfhSaeYBe8vkEFjhSWDXacsPz9LreP6HCR3xpzOpR9N/2ZGtppiQ9SN
  SbFMFZ9EeUS/d+Wzdfq5vSX2X9nsuwDRbhv2EYmbPcfpAAAAgDez0GCF/gmw1tuL4EgPLH
  uwvQmUH8tYgUBtXJesGLA1YUJdRkBUDKaOZj+LPY0MI1dT328br1TIxK8sYGbfjGTrmNwk
  Q2Ga4geoppqE/LkuEUDu5pPeXOoH+7BwKN3KtQ3/L8jZI3jUKvBwAbyQn/auslqnlAkYtx
  9jITJ0tOQOAAAAgQDcH7swgrButn+rdyRVIfS1xP8qBguHR1vvzcYSJM7jAWfdUN11Ob+W
  1AOhE3i2pZg36duVS+WkzX0BX3POXaB6wFRVyysm/N9JewyEy+T/gut184zJt8Gt2HtCX1
  4IWVth+/2sNexdjMEgP5FiLFlaGVdgh1Xxylr/VDddp0NOewAAAIEA1zap/WjPIdstpK90
  c8JPEWbAVVVxYwPn8V23nSi+AP/P9zPhpxghC/5ZEK5xBD9F52FAZT3OXxlU36kn1V9Bfj
  lKnxOX+4TghB8cA2BNHjVrn5/6+k7PLYRGWMdRF98pnP7lEYImrtqQSaFj0NV1kSLP2qz7
  2ti3YcTndr4Yj7UAAAAWaWFud2FoYmVATWFjLmZyaXR6LmJveAECAwQF
  -----END OPENSSH PRIVATE KEY-----
  EOT
}

output "decrypted" {
  value = rsadecrypt(local.ciphertext, local.private_key)
}
