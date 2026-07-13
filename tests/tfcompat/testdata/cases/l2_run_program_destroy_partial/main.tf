resource "partialdestroy_resource" "blocker" {
  fail_delete = true
  zones       = ["b"]
}

# The reference to blocker orders this resource after blocker on create, and so
# before blocker on destroy — without relying on depends_on. On the first
# destroy it is deleted, then blocker's delete fails and aborts the destroy,
# leaving this resource absent from state.
resource "partialdestroy_resource" "gone" {
  zones = [partialdestroy_resource.blocker.zones[0]]
}

# On a --run-program destroy after the partial destroy, this indexes a resource
# that is no longer in state.
output "gone_zone" {
  value = partialdestroy_resource.gone.zones[0]
}
