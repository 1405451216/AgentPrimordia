# AgentPrimordia Terraform 变量定义

variable "cluster_name" {
  description = "集群名称前缀"
  type        = string
  default     = "ap"
}

variable "node_count" {
  description = "Agent 节点数量"
  type        = number
  default     = 3

  validation {
    condition     = var.node_count >= 1 && var.node_count <= 20
    error_message = "node_count 必须在 1-20 之间"
  }
}

variable "ap_image" {
  description = "AgentPrimordia 镜像"
  type        = string
  default     = "ghcr.io/agentprimordia/ap:v3.1.0"
}

variable "network_subnet" {
  description = "集群网络子网"
  type        = string
  default     = "172.28.0.0/16"
}

variable "etcd_port" {
  description = "etcd 对外端口"
  type        = number
  default     = 2379
}

variable "base_http_port" {
  description = "Agent 节点 HTTP 起始端口（node-1 使用此端口）"
  type        = number
  default     = 8081
}

variable "base_metrics_port" {
  description = "Agent 节点 Metrics 起始端口"
  type        = number
  default     = 9091
}

variable "container_memory_mb" {
  description = "每个 Agent 节点内存限制（MB）"
  type        = number
  default     = 1024
}

variable "log_level" {
  description = "日志级别"
  type        = string
  default     = "info"

  validation {
    condition     = contains(["debug", "info", "warn", "error"], var.log_level)
    error_message = "log_level 必须是 debug/info/warn/error 之一"
  }
}

variable "enable_monitoring" {
  description = "是否启用 Prometheus + Grafana 监控"
  type        = bool
  default     = true
}

variable "prometheus_config_path" {
  description = "Prometheus 配置文件路径"
  type        = string
  default     = "../prometheus/prometheus.yml"
}

variable "prometheus_port" {
  description = "Prometheus 对外端口"
  type        = number
  default     = 9090
}

variable "grafana_port" {
  description = "Grafana 对外端口"
  type        = number
  default     = 3001
}

variable "grafana_admin_password" {
  description = "Grafana 管理员密码"
  type        = string
  default     = "agentprimordia"
  sensitive   = true
}
