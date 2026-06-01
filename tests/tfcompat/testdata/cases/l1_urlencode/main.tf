# Terraform's `urlencode` applies query-string URL encoding (Go's
# url.QueryEscape): spaces become `+` and non-ASCII characters are first encoded
# as UTF-8 and then percent-encoded byte by byte.
output "space" {
  value = urlencode("a b c")
}

output "reserved" {
  value = urlencode("a/b?c=d&e")
}

output "plus" {
  value = urlencode("a+b")
}

output "unicode" {
  value = urlencode("café")
}

output "emoji" {
  value = urlencode("hi 😀")
}
