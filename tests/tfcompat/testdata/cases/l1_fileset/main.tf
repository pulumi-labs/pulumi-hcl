locals {
  assets = "${path.module}/assets"
}

# Recursive `**` matches files at every depth.
output "recursive_all" {
  value = fileset(local.assets, "**")
}

# `**/*.txt` recurses and filters by extension.
output "recursive_txt" {
  value = fileset(local.assets, "**/*.txt")
}

# A single `*` matches only entries directly inside the directory, and
# excludes the `nested` subdirectory because fileset returns files only.
output "top_level" {
  value = fileset(local.assets, "*")
}
