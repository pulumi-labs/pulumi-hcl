# The resource is gone from the configuration, so it is destroyed as an orphan.
# The destroy-time provisioner went away with the resource block and does not
# run; the apply succeeds. Nothing here declares a provisioner, so the run also
# covers a program that must service a hook it never declares.
output "gone" { value = "gone" }
