terraform {
  required_providers {
    fog = {
      source  = "saathvikk/fog"
      version = "0.1.0"
    }
  }
}

# 1. Prepare Node: Install dependencies and fetch the Python script
resource "terraform_data" "prepare_node" {
  input = {
    host         = var.node_ip
    user         = var.node_user
    service_name = var.service_name
  }

  triggers_replace = [var.script_url]

  provisioner "remote-exec" {
    connection {
      type = "ssh"
      user = var.node_user
      host = var.node_ip
    }

    inline = [
      "sudo apt-get update && sudo apt-get install -y python3-pip",
      "sudo pip3 install paho-mqtt RPi.GPIO --break-system-packages || sudo pip3 install paho-mqtt RPi.GPIO",
      "mkdir -p /home/${var.node_user}/services",
      "wget -O /home/${var.node_user}/services/${var.service_name}.py ${var.script_url}"
    ]
  }

  provisioner "remote-exec" {
    when = destroy
    connection {
      type = "ssh"
      user = self.input.user
      host = self.input.host
    }
    inline = [
      "rm -f /home/${self.input.user}/services/${self.input.service_name}.py"
    ]
  }
}

# 2. Manage Service: Use the custom provider with hash-based drift detection
resource "fog_systemd_service" "daemon" {
  # Wait for the python script and dependencies to be installed first
  depends_on = [terraform_data.prepare_node]

  node_address = var.node_ip
  node_user    = var.node_user
  service_name = var.service_name
  
  # Command to run the script we just downloaded
  exec_start   = "/usr/bin/python3 /home/${var.node_user}/services/${var.service_name}.py"

  # Native environment variable injection! No more `echo` or `sed`.
  environment = {
    MQTT_BROKER = split(":", var.broker_url)[0]
    MQTT_PORT   = split(":", var.broker_url)[1]
  }
}
