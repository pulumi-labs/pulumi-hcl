# `a` defers its read because it `depends_on` a resource that is about to be
# created. `b` only names `a`, and a `depends_on` edge onto a data source is
# never on its own a reason to defer — a read has no side effects. But `b`
# carries a custom condition, and a condition can only be made to pass by an
# upstream change, so the deferral decision widens to `b`'s indirect
# dependencies: `pending_thing.thing` sits behind `a`, so `b` defers too.
resource "pending_thing" "thing" {
  name = "widget"
}

data "pending_lookup" "a" {
  name = "widget"

  depends_on = [pending_thing.thing]
}

data "pending_lookup" "b" {
  name = "widget"

  depends_on = [data.pending_lookup.a]

  lifecycle {
    postcondition {
      condition     = self.result != ""
      error_message = "the lookup returned no result"
    }
  }
}

output "looked_up" {
  value = data.pending_lookup.b.result
}
