# AgentPrimordia Terraform Module — 云基础设施部署
#
# 用法:
#   module "agentprimordia" {
#     source        = "./deploy/terraform"
#     cluster_name  = "ap-prod"
#     node_count    = 3
#     instance_type = "t3.medium"
#   }

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0"
    }
  }
}

# ─── 网络 ─────────────────────────────────────────────────────
resource "docker_network" "ap_cluster" {
  name = "${var.cluster_name}-network"
  driver = "bridge"

  ipam_config {
    subnet = var.network_subnet
  }
}

# ─── etcd ─────────────────────────────────────────────────────
resource "docker_container" "etcd" {
  name  = "${var.cluster_name}-etcd"
  image = "bitnami/etcd:3.5"

  env = [
    "ALLOW_NONE_AUTHENTICATION=yes",
    "ETCD_ADVERTISE_CLIENT_URLS=http://${var.cluster_name}-etcd:2379",
    "ETCD_LISTEN_CLIENT_URLS=http://0.0.0.0:2379",
  ]

  ports {
    internal = 2379
    external = var.etcd_port
  }

  networks_advanced {
    name = docker_network.ap_cluster.name
  }

  healthcheck {
    test     = ["CMD", "etcdctl", "endpoint", "health"]
    interval = "5s"
    timeout  = "3s"
    retries  = 10
  }

  restart = "unless-stopped"
}

# ─── Agent 节点 ───────────────────────────────────────────────
resource "docker_container" "ap_nodes" {
  count = var.node_count

  name  = "${var.cluster_name}-node-${count.index + 1}"
  image = var.ap_image

  env = concat([
    "AP_NODE_ID=node-${count.index + 1}",
    "AP_LISTEN_ADDR=:8080",
    "AP_CLUSTER_ENABLED=true",
    "AP_CLUSTER_DISCOVERY=etcd",
    "AP_ETCD_ENDPOINTS=http://${var.cluster_name}-etcd:2379",
    "AP_METRICS_ENABLED=true",
    "AP_METRICS_PORT=9090",
    "AP_LOG_LEVEL=${var.log_level}",
  ], count.index == 0 ? ["AP_CLUSTER_BOOTSTRAP=true"] : [
    "AP_CLUSTER_SEEDS=${var.cluster_name}-node-1:8080",
  ])

  ports {
    internal = 8080
    external = var.base_http_port + count.index
  }

  ports {
    internal = 9090
    external = var.base_metrics_port + count.index
  }

  networks_advanced {
    name = docker_network.ap_cluster.name
  }

  depends_on = [docker_container.etcd]

  restart = "unless-stopped"

  memory = var.container_memory_mb
}

# ─── Prometheus（可选）────────────────────────────────────────
resource "docker_container" "prometheus" {
  count = var.enable_monitoring ? 1 : 0

  name  = "${var.cluster_name}-prometheus"
  image = "prom/prometheus:v2.53.0"

  volumes {
    container_path = "/etc/prometheus/prometheus.yml"
    host_path      = var.prometheus_config_path
    read_only      = true
  }

  ports {
    internal = 9090
    external = var.prometheus_port
  }

  networks_advanced {
    name = docker_network.ap_cluster.name
  }

  depends_on = [docker_container.ap_nodes]

  restart = "unless-stopped"
}

# ─── Grafana（可选）───────────────────────────────────────────
resource "docker_container" "grafana" {
  count = var.enable_monitoring ? 1 : 0

  name  = "${var.cluster_name}-grafana"
  image = "grafana/grafana:11.1.0"

  env = [
    "GF_SECURITY_ADMIN_PASSWORD=${var.grafana_admin_password}",
    "GF_USERS_ALLOW_SIGN_UP=false",
  ]

  ports {
    internal = 3000
    external = var.grafana_port
  }

  networks_advanced {
    name = docker_network.ap_cluster.name
  }

  depends_on = [docker_container.prometheus]

  restart = "unless-stopped"
}
