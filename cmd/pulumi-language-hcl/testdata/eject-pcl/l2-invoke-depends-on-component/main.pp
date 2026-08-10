resource "target" "component:index:ComponentCustomRefOutput" {
  value = "checked"
}

output "echoed" {
  value = invoke("component:index:identity", {
    input = "reachable"
    }, {
    dependsOn = [target]
  }).result
}

