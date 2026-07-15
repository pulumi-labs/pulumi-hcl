# module.consumer depends_on module.producer with no data reference between
# them, so only the explicit depends_on can order the two order_resource
# creates: the recorded sequence must be [create producer, create consumer].
# The producer's create is delayed, so a missing depends_on edge lets the
# consumer's create record first and flips the order deterministically.
module "producer" {
  source = "./producer"
}

module "consumer" {
  source     = "./consumer"
  depends_on = [module.producer]
}
