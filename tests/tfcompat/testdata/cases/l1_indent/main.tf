# Terraform's `indent` adds the given number of spaces after every newline,
# which leaves the first line untouched and still pads otherwise-blank lines.
# The surrounding brackets make the leading whitespace of each line visible in
# the stack output.
output "two_lines" {
  value = "[${indent(2, "hello\nworld")}]"
}

output "blank_line" {
  value = "[${indent(2, "a\n\nb")}]"
}

output "list_block" {
  value = "items: ${indent(4, "[\n  a,\n  b,\n]")}"
}

output "single_line" {
  value = "[${indent(4, "no newline")}]"
}
