# A count resource indexed by a value computed from a data source
# (`r0[tonumber(data.order_data.idx.result)]`). The index is dynamic, so the
# reference is to the whole resource: `r1` must wait for every `r0` instance,
# including the delayed `r0[1]`, even though the resolved index selects
# `r0[0]`. The data source's config is fully known, so it reads at plan time,
# ahead of every create: the recorded order is [read idx, r0[0], r0[1], r1].
# `r0[1]`'s create is delayed, so a runtime that narrowed the edge to only the
# selected instance would record `r1` ahead of `r0[1]` — a deterministic order
# flip. The output pins that the dynamic index still resolves to r0[0]'s value.
data "order_data" "idx" {
  name = "0"
}

resource "order_resource" "r0" {
  count        = 2
  name         = "r0-${count.index}"
  delay_create = count.index == 1
}

resource "order_resource" "r1" {
  name = "r1-${order_resource.r0[tonumber(data.order_data.idx.result)].result}"
}

output "r1_result" {
  value = order_resource.r1.result
}
