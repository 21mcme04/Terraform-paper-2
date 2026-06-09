variable "master_node" {
  type = object({ ip = string, user = string })
}

variable "worker_nodes" {
  type = map(object({ ip = string, user = string }))
}