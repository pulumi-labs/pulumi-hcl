# base64decode of a valid base64 string whose decoded bytes are NOT valid
# UTF-8. can() isolates the difference into a comparable boolean output that
# both runtimes can produce.
output "can_ff" { value = can(base64decode("/w==")) }

output "can_80" { value = can(base64decode("gA==")) }

output "can_multi" { value = can(base64decode("//79")) }

# A control: valid UTF-8 decodes cleanly on both.
output "can_hello" { value = can(base64decode("SGVsbG8=")) }
