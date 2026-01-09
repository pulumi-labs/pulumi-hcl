# AWS Webserver Example
# This creates a simple web server on AWS, equivalent to:
# https://github.com/pulumi/examples/tree/master/aws-ts-webserver

# Get the most recent Amazon Linux 2023 AMI
data "aws_ami" "amazon_linux" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }
}

# Create a security group allowing HTTP traffic
resource "aws_security_group" "web_secgrp" {
  description = "Enable HTTP access"

  ingress {
    protocol    = "tcp"
    from_port   = 80
    to_port     = 80
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# User data script to start a simple web server
locals {
  user_data = <<-EOF
    #!/bin/bash
    echo "Hello, World!" > index.html
    nohup python3 -m http.server 80 &
  EOF
}

# Create the EC2 instance
resource "aws_instance" "web_server" {
  ami                    = data.aws_ami.amazon_linux.id
  instance_type          = "t2.micro"
  vpc_security_group_ids = [aws_security_group.web_secgrp.id]
  user_data              = local.user_data

  tags = {
    Name = "web-server-www"
  }
}

# Export the public IP and hostname
output "public_ip" {
  value       = aws_instance.web_server.public_ip
  description = "The public IP address of the web server"
}

output "public_hostname" {
  value       = aws_instance.web_server.public_dns
  description = "The public DNS hostname of the web server"
}
