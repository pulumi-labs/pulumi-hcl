# A template that is a single `${...}` interpolation with no surrounding literal
# text evaluates to the interpolated value's own type — list, number, bool,
# object — rather than a stringified form. A template with any literal text
# still renders to a string. `templatefile` and `templatestring` behave the
# same way (`value.tftpl` is exactly `${a}`; `text.tftpl` has literal text).

output "file_list"   { value = templatefile("${path.module}/value.tftpl", { a = ["foo", "bar"] }) }
output "file_number" { value = templatefile("${path.module}/value.tftpl", { a = 42 }) }
output "file_bool"   { value = templatefile("${path.module}/value.tftpl", { a = true }) }

output "string_list"   { value = templatestring("$${a}", { a = ["x", "y"] }) }
output "string_object" { value = templatestring("$${a}", { a = { k = 1 } }) }

# Literal text forces a string result on both functions.
output "file_text"   { value = templatefile("${path.module}/text.tftpl", { a = 42 }) }
output "string_text" { value = templatestring("n=$${a}", { a = 42 }) }
