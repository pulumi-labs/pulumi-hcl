output "regex" {
  value = replace("hello world", "/o./", "X")
}

output "anchored" {
  value = replace("foo bar foo", "/^foo/", "Z")
}

output "digits" {
  value = replace("a1b2c3", "/[0-9]/", "_")
}

output "slashes" {
  value = replace("a/b/c", "/b/", "X")
}

output "literal" {
  value = replace("a.b.c", ".", "-")
}
