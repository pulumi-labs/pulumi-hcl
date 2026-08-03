// A multi-argument invoke passes its arguments positionally and omits the ones the program leaves
// out, so parenting it must not displace the options bag into an argument slot.
greeting = invoke("multi-argument-invoke:index:multiArgumentInvoke", "hello", null)

output "result" {
  value = invoke("config:index:getConfig", {
    text = greeting.result
  }).text
}

