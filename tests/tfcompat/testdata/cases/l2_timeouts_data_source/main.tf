data "timeoutable_data" "test" {
  timeouts {
    read = "1m"
  }
}

output "read_timeout" {
  value = data.timeoutable_data.test.timeouts.read
}

output "timeouts" {
  value = data.timeoutable_data.test.timeouts
}
