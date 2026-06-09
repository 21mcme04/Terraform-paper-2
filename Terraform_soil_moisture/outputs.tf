output "master_ip" {
  value = var.master_node.ip
}

output "status" {
  value = "Deployment complete. Publisher, Actuator, and Broker are running."
}