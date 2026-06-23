terraform {
  required_providers {
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
  }
}

data "primitive_invoke" "invokeResult" {
  boolean      = false
  float        = 2.17
  integer      = -12
  string       = "adversarial"
  number_array = [0, 1]
  boolean_map = {
    "__type"                                                                                                                                               = true
    "__internal"                                                                                                                                           = false
    "__provider"                                                                                                                                           = true
    "__version"                                                                                                                                            = false
    ""                                                                                                                                                     = true
    "Some $${common} \"characters\" 'that' need escaping: \\ (backslash), \t (tab), \u001b (escape), \u0007 (bell), \u0000 (null), \U000e0021 (tag space)" = false
    "Format and glob specifiers: %percent ...ellipsis {open }close *asterisk ?question ,comma &&and ||or !not =>arrow ==equal :colon /slash"               = true
  }
}

resource "primitive_resource" "res" {
  lifecycle {
    create_before_destroy = true
  }
  boolean      = false
  float        = 2.17
  integer      = -12
  string       = "adversarial"
  number_array = [0, 1]
  boolean_map = {
    "__type"                                                                                                                                               = true
    "__internal"                                                                                                                                           = false
    "__provider"                                                                                                                                           = true
    "__version"                                                                                                                                            = false
    ""                                                                                                                                                     = true
    "Some $${common} \"characters\" 'that' need escaping: \\ (backslash), \t (tab), \u001b (escape), \u0007 (bell), \u0000 (null), \U000e0021 (tag space)" = false
    "Format and glob specifiers: %percent ...ellipsis {open }close *asterisk ?question ,comma &&and ||or !not =>arrow ==equal :colon /slash"               = true
  }
}
output "resourceBooleanMap" {
  value = primitive_resource.res.boolean_map
}
output "invokeBooleanMap" {
  value = data.primitive_invoke.invokeResult.boolean_map
}
