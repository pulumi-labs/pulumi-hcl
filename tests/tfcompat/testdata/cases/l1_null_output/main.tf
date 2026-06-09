output "present" {
  value = "hello"
}

output "absent_when_null" {
  value = null
}

# Only wholly-null outputs are dropped: an output whose value merely contains a
# null must still be emitted, with the inner null preserved.
output "object_with_inner_null" {
  value = { a = "x", b = null }
}
