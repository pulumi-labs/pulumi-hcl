variable "tags" {
  type    = map(string)
  default = {}
}

variable "subnets" {
  type    = list(string)
  default = []
}

variable "ports" {
  type    = set(number)
  default = []
}

variable "settings" {
  type = object({
    cidr   = string
    public = bool
  })
  default = {
    cidr   = "10.0.0.0/16"
    public = false
  }
}

output "subnet_ids" {
  value = var.subnets
}

output "tag_map" {
  value = var.tags
}
