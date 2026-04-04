terraform {
  required_providers {
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
  }
}

variable "plainBool" {
  type = bool
}
variable "plainNumber" {
  type = number
}
variable "plainInteger" {
  type = number
}
variable "plainString" {
  type = string
}
variable "secretBool" {
  type = bool
}
variable "secretNumber" {
  type = number
}
variable "secretInteger" {
  type = number
}
variable "secretString" {
  type = string
}
module "plain" {
  source  = "./primitiveComponent"
  boolean = var.plainBool
  float   = var.plainNumber
  integer = var.plainInteger
  string  = var.plainString
}
module "secret" {
  source  = "./primitiveComponent"
  boolean = var.secretBool
  float   = var.secretNumber
  integer = var.secretInteger
  string  = var.secretString
}
