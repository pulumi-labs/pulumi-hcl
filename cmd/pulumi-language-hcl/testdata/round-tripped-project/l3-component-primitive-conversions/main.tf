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
variable "plainNumericString" {
  type = string
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
variable "secretNumericString" {
  type = string
}
module "plainValues" {
  source  = "./conversionComponent"
  boolean = var.plainString
  float   = var.plainInteger
  integer = var.plainNumericString
  string  = var.plainNumber
}
module "secretValues" {
  source  = "./conversionComponent"
  boolean = var.secretString
  float   = var.secretInteger
  integer = var.secretNumericString
  string  = var.secretNumber
}
