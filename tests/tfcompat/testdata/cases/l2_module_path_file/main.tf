# A file-reading function called from inside a child module must resolve a relative path
# the way OpenTofu does — against the root module / process working directory — so that a
# `"${path.module}/<file>"` argument locates the file inside the declaring module. The
# child invokes every affected function against mod/aux.txt (and mod/tmpl.tftpl); both
# runtimes must compute identical outputs.
module "m" {
  source = "./mod"
}

output "file" {
  value = module.m.file
}

output "fileexists" {
  value = module.m.fileexists
}

output "filebase64" {
  value = module.m.filebase64
}

output "filemd5" {
  value = module.m.filemd5
}

output "filesha1" {
  value = module.m.filesha1
}

output "filesha256" {
  value = module.m.filesha256
}

output "filesha512" {
  value = module.m.filesha512
}

output "filebase64sha256" {
  value = module.m.filebase64sha256
}

output "filebase64sha512" {
  value = module.m.filebase64sha512
}

output "fileset" {
  value = module.m.fileset
}

output "templatefile" {
  value = module.m.templatefile
}
