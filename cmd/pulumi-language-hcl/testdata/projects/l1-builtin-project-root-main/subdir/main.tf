output "rootDirectoryOutput" {
  value = path.cwd
}
output "workingDirectoryOutput" {
  value = abspath(path.root)
}
