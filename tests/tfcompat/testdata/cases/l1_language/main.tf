# A language block declares compatibility with several implementations at
# once: each implementation honors its own argument and silently ignores the
# rest, including arguments whose values are not version constraints at all.
language {
  compatible_with {
    opentofu       = ">= 1.12.0"
    pulumi         = ">= 3.0.0"
    other_software = ["not", "a", "version"]
  }
}

output "attr" {
  value = "ok"
}
