resource "terraform_data" "deploy_k3s_stack" {
  triggers_replace = [var.manifest_url]

  provisioner "local-exec" {
    command = <<EOT
      export KUBECONFIG=${var.kubeconfig_path}
      kubectl apply -f "${var.manifest_url}"
    EOT
  }

  # Cleanup on destroy
  provisioner "local-exec" {
    when    = destroy
    command = "export KUBECONFIG=./kubeconfig.yaml && kubectl delete -f ${self.triggers_replace[0]} --ignore-not-found=true"
  }
}