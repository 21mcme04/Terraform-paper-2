resource "terraform_data" "k3s_master" {
  input = {
    host = var.master_node.ip
    user = var.master_node.user
  }

  triggers_replace = [var.master_node.ip]

  provisioner "file" {
    connection {
      type = "ssh"
      user = var.master_node.user
      host = var.master_node.ip
    }
    source      = "${path.root}/scripts/install_k3s_master.sh"
    destination = "/tmp/install_k3s_master.sh"
  }

  provisioner "remote-exec" {
    connection {
      type = "ssh"
      user = var.master_node.user
      host = var.master_node.ip
    }
    inline = [
      "chmod +x /tmp/install_k3s_master.sh",
      "/tmp/install_k3s_master.sh"
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
      "if [ -f /usr/local/bin/k3s-uninstall.sh ]; then /usr/local/bin/k3s-uninstall.sh; fi"
    ]
  }
}

resource "terraform_data" "fetch_config" {
  depends_on = [terraform_data.k3s_master]
  triggers_replace = [var.master_node.ip]

  provisioner "local-exec" {
    command = "scp -o StrictHostKeyChecking=no ${var.master_node.user}@${var.master_node.ip}:/etc/rancher/k3s/k3s.yaml ${path.root}/kubeconfig.yaml && sed -i 's/127.0.0.1/${var.master_node.ip}/g' ${path.root}/kubeconfig.yaml"
  }
  
  provisioner "local-exec" {
    command = "ssh -o StrictHostKeyChecking=no ${var.master_node.user}@${var.master_node.ip} 'sudo cat /var/lib/rancher/k3s/server/node-token' > ${path.root}/k3s_token.txt"
  }

  provisioner "local-exec" {
    when    = destroy
    command = "rm -f ${path.root}/kubeconfig.yaml ${path.root}/k3s_token.txt"
  }
}

data "local_file" "k3s_token" {
  depends_on = [terraform_data.fetch_config]
  filename   = "${path.root}/k3s_token.txt"
}

resource "terraform_data" "k3s_workers" {
  for_each = var.worker_nodes
  depends_on = [data.local_file.k3s_token]

  input = {
    host = each.value.ip
    user = each.value.user
  }

  triggers_replace = [each.value.ip]
  
  provisioner "file" {
    connection {
      type = "ssh"
      user = each.value.user
      host = each.value.ip
    }
    source      = "${path.root}/scripts/install_k3s_worker.sh"
    destination = "/tmp/install_k3s_worker.sh"
  }

  provisioner "remote-exec" {
    connection {
      type = "ssh"
      user = each.value.user
      host = each.value.ip
    }
    inline = [
      "chmod +x /tmp/install_k3s_worker.sh",
      "export MASTER_IP=${var.master_node.ip}",
      "export K3S_TOKEN=${trimspace(data.local_file.k3s_token.content)}",
      "/tmp/install_k3s_worker.sh"
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
      "if [ -f /usr/local/bin/k3s-agent-uninstall.sh ]; then /usr/local/bin/k3s-agent-uninstall.sh; fi"
    ]
  }
}