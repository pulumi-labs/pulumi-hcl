variable "handler" {
}

# var.handler is a whole resource the calling program passed by reference. Its
# fields are read here, inside the module, which requires the runtime to have
# fetched the referenced resource's state.
output "handler_value" {
  value = var.handler.value
}

output "handler_id" {
  value = var.handler.id
}
