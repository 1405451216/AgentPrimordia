# AgentPrimordia Terraform 输出

output "cluster_name" {
  description = "集群名称"
  value       = var.cluster_name
}

output "node_endpoints" {
  description = "Agent 节点 HTTP 端点列表"
  value = [
    for i in range(var.node_count) :
    "http://localhost:${var.base_http_port + i}"
  ]
}

output "metrics_endpoints" {
  description = "Agent 节点 Metrics 端点列表"
  value = [
    for i in range(var.node_count) :
    "http://localhost:${var.base_metrics_port + i}/metrics"
  ]
}

output "etcd_endpoint" {
  description = "etcd 端点"
  value       = "http://localhost:${var.etcd_port}"
}

output "prometheus_endpoint" {
  description = "Prometheus 端点"
  value       = var.enable_monitoring ? "http://localhost:${var.prometheus_port}" : ""
}

output "grafana_endpoint" {
  description = "Grafana 端点"
  value       = var.enable_monitoring ? "http://localhost:${var.grafana_port}" : ""
}

output "network_name" {
  description = "Docker 网络名称"
  value       = docker_network.ap_cluster.name
}
