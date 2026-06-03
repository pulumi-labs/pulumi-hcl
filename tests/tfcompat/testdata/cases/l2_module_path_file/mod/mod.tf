# Every file-reading function resolves its `${path.module}`-relative argument
# against the root module directory, so each must locate mod/aux.txt (and
# mod/tmpl.tftpl) rather than looking one directory too deep. fileset and
# templatefile take the module directory / a module-relative path directly.
locals {
  aux = "${path.module}/aux.txt"
}

output "file" {
  value = file(local.aux)
}

output "fileexists" {
  value = fileexists(local.aux)
}

output "filebase64" {
  value = filebase64(local.aux)
}

output "filemd5" {
  value = filemd5(local.aux)
}

output "filesha1" {
  value = filesha1(local.aux)
}

output "filesha256" {
  value = filesha256(local.aux)
}

output "filesha512" {
  value = filesha512(local.aux)
}

output "filebase64sha256" {
  value = filebase64sha256(local.aux)
}

output "filebase64sha512" {
  value = filebase64sha512(local.aux)
}

output "fileset" {
  value = join(",", sort(tolist(fileset(path.module, "*.txt"))))
}

output "templatefile" {
  value = templatefile("${path.module}/tmpl.tftpl", { name = "world" })
}
