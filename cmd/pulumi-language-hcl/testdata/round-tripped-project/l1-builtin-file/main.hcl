locals {
  fileContent = file("testfile.txt")
}
locals {
  fileB64 = filebase64("testfile.txt")
}
locals {
  fileSha = filebase64sha256("testfile.txt")
}
output "fileContent" {
  value = local.fileContent
}
output "fileB64" {
  value = local.fileB64
}
output "fileSha" {
  value = local.fileSha
}
