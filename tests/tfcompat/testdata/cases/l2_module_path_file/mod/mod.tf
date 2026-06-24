# Every file-reading function resolves its `${path.module}`-relative argument
# against the root module directory, so each must locate mod/data.txt (and
# mod/tmpl.tftpl) rather than looking one directory too deep. fileset and
# templatefile take the module directory / a module-relative path directly.
locals {
  data = "${path.module}/data.txt"
}

output "file" {
  value = file(local.data)
}

output "fileexists" {
  value = fileexists(local.data)
}

output "filebase64" {
  value = filebase64(local.data)
}

output "filemd5" {
  value = filemd5(local.data)
}

output "filesha1" {
  value = filesha1(local.data)
}

output "filesha256" {
  value = filesha256(local.data)
}

output "filesha512" {
  value = filesha512(local.data)
}

output "filebase64sha256" {
  value = filebase64sha256(local.data)
}

output "filebase64sha512" {
  value = filebase64sha512(local.data)
}

output "fileset" {
  value = join(",", sort(tolist(fileset(path.module, "*.txt"))))
}

output "templatefile" {
  value = templatefile("${path.module}/tmpl.tftpl", { name = "world" })
}
