# A reverse chain across count resources: r0 depends on r1[0], r1 on r2[0],
# r2 on r3[0], and r3 depends on nothing, so creates run in reverse
# declaration order. r3's second instance is delayed: the instance-addressed
# edges let the whole chain complete before r3[1], recording
# [r3-0, r2, r1, r0, r3-1]. A runtime that widens any hop to the whole
# resource waits for the delayed r3[1] mid-chain — a deterministic flip.
resource "order_resource" "r0" {
  count      = 1
  name       = "r0"
  depends_on = [order_resource.r1[0]]
}

resource "order_resource" "r1" {
  count      = 1
  name       = "r1"
  depends_on = [order_resource.r2[0]]
}

resource "order_resource" "r2" {
  count      = 1
  name       = "r2"
  depends_on = [order_resource.r3[0]]
}

resource "order_resource" "r3" {
  count        = 2
  name         = "r3-${count.index}"
  delay_create = count.index == 1
}

output "r0_result" {
  value = order_resource.r0[0].result
}
