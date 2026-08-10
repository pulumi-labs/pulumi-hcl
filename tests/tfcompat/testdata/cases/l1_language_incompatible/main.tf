# Both implementations reject their own constraint when it excludes the
# running version.
language {
  compatible_with {
    opentofu = "< 1.0.0"
    pulumi   = "< 1.0.0"
  }
}

output "attr" {
  value = "unreachable"
}
