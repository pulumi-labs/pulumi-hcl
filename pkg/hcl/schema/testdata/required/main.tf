variable "zone" {
  type     = string
  nullable = false
}

variable "region" {
  type     = string
  nullable = false
}

variable "account" {
  type     = string
  nullable = false
}

variable "optional" {
  type    = string
  default = "ok"
}
