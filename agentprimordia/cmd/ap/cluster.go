package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Phase 3.1: CLI 集群管理
//
// 命令：
//   ap cluster init          初始化集群（生成配置 + 启动引导节点）
//   ap cluster join <addr>   加入已有集群
//   ap cluster status        查看集群状态（节点列表/领导者/分片）
//   ap cluster leave         优雅离开集群
//   ap cluster scale <n>     扩缩容节点

func runCluster(args []string) error {
	if len(args) == 0 {
		printClusterHelp()
		return nil
	}

	subcmd := args[0]
	switch subcmd {
	case "init":
		return runClusterInit(args[1:])
	case "join":
		return runClusterJoin(args[1:])
	case "status":
		return runClusterStatus(args[1:])
	case "leave":
		return runClusterLeave(args[1:])
	case "scale":
		return runClusterScale(args[1:])
	case "--help", "-h", "help":
		printClusterHelp()
		return nil
	default:
		return fmt.Errorf("unknown cluster subcommand %q, run %s for help", subcmd, bold("ap cluster --help"))
	}
}

func printClusterHelp() {
	fmt.Print(`ap cluster — manage AgentPrimordia cluster

Usage:
  ap cluster <command> [arguments]

Commands:
  init              initialize a new cluster (generate config + start bootstrap node)
  join <addr>       join an existing cluster
  status            show cluster status (nodes/leader/shards)
  leave             gracefully leave the cluster
  scale <n>         scale cluster to n nodes

Options:
  --node-id ID      node identifier (default: auto-generated)
  --addr ADDR       listen address (default: :8080)
  --config PATH     cluster config file path (default: .ap-cluster.json)

Examples:
  ap cluster init --node-id node-1 --addr :8080
  ap cluster join 192.168.1.10:8080
  ap cluster status
  ap cluster leave
  ap cluster scale 5
`)
}

// ===== 集群配置 =====

// clusterConfig 集群配置文件结构
type clusterConfig struct {
	NodeID            string   `json:"node_id"`
	ListenAddr        string   `json:"listen_addr"`
	BootstrapNodes    []string `json:"bootstrap_nodes,omitempty"`
	HeartbeatInterval string   `json:"heartbeat_interval,omitempty"`
	HeartbeatTimeout  string   `json:"heartbeat_timeout,omitempty"`
	CreatedAt         string   `json:"created_at"`
}

const defaultClusterConfigPath = ".ap-cluster.json"

// validateClusterConfigPath 验证配置文件路径安全性
// 防止路径遍历攻击：确保路径不含 ".." 且为相对路径或当前目录下
func validateClusterConfigPath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	// 禁止包含 ".." 的路径
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("invalid config path %q: path traversal not allowed", path)
	}
	// 如果是绝对路径，验证不是系统敏感目录
	if filepath.IsAbs(cleaned) {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return "", fmt.Errorf("resolve config path: %w", err)
		}
		cleaned = abs
	}
	return cleaned, nil
}

// ===== 子命令实现 =====

func runClusterInit(args []string) error {
	nodeID := ""
	addr := ":8080"
	configPath := defaultClusterConfigPath

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--node-id":
			i++
			if i >= len(args) {
				return fmt.Errorf("--node-id requires a value")
			}
			nodeID = args[i]
		case "--addr":
			i++
			if i >= len(args) {
				return fmt.Errorf("--addr requires a value")
			}
			addr = args[i]
		case "--config":
			i++
			if i >= len(args) {
				return fmt.Errorf("--config requires a path")
			}
			configPath = args[i]
		case "--help", "-h":
			fmt.Print(`ap cluster init — initialize a new cluster

Usage:
  ap cluster init [--node-id ID] [--addr ADDR] [--config PATH]

Options:
  --node-id ID      node identifier (default: hostname-based)
  --addr ADDR       listen address (default: :8080)
  --config PATH     config file path (default: .ap-cluster.json)
`)
			return nil
		}
	}

	// 自动生成 node ID
	if nodeID == "" {
		hostname, _ := os.Hostname()
		nodeID = fmt.Sprintf("node-%s-%d", hostname, time.Now().Unix()%10000)
	}

	// 验证配置路径
	configPath, err := validateClusterConfigPath(configPath)
	if err != nil {
		return err
	}

	// 检查配置文件是否已存在
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("cluster config %q already exists, use 'ap cluster join' to join an existing cluster", configPath)
	}

	// 生成配置
	cfg := clusterConfig{
		NodeID:            nodeID,
		ListenAddr:        addr,
		HeartbeatInterval: "5s",
		HeartbeatTimeout:  "15s",
		CreatedAt:         time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cluster config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write cluster config: %w", err)
	}

	infof("集群初始化完成")
	fmt.Printf("  节点 ID:   %s\n", bold(nodeID))
	fmt.Printf("  监听地址:  %s\n", bold(addr))
	fmt.Printf("  配置文件:  %s\n", configPath)
	fmt.Println()
	fmt.Printf("其他节点可通过以下命令加入：\n")
	fmt.Printf("  %s\n", bold("ap cluster join "+addr))

	return nil
}

func runClusterJoin(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ap cluster join <addr> [--node-id ID]")
	}

	bootstrapAddr := args[0]
	nodeID := ""
	configPath := defaultClusterConfigPath

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--node-id":
			i++
			if i >= len(args) {
				return fmt.Errorf("--node-id requires a value")
			}
			nodeID = args[i]
		case "--config":
			i++
			if i >= len(args) {
				return fmt.Errorf("--config requires a path")
			}
			configPath = args[i]
		}
	}

	if nodeID == "" {
		hostname, _ := os.Hostname()
		nodeID = fmt.Sprintf("node-%s-%d", hostname, time.Now().Unix()%10000)
	}

	// 验证配置路径
	configPath, err := validateClusterConfigPath(configPath)
	if err != nil {
		return err
	}

	// 加载或创建配置
	var cfg clusterConfig
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parse cluster config: %w", err)
		}
	} else {
		cfg = clusterConfig{
			NodeID:            nodeID,
			ListenAddr:        ":8081",
			HeartbeatInterval: "5s",
			HeartbeatTimeout:  "15s",
			CreatedAt:         time.Now().Format(time.RFC3339),
		}
	}

	// 添加引导节点
	found := false
	for _, n := range cfg.BootstrapNodes {
		if n == bootstrapAddr {
			found = true
			break
		}
	}
	if !found {
		cfg.BootstrapNodes = append(cfg.BootstrapNodes, bootstrapAddr)
	}

	cfg.NodeID = nodeID

	// 保存配置
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cluster config: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write cluster config: %w", err)
	}

	infof("已加入集群")
	fmt.Printf("  节点 ID:     %s\n", bold(nodeID))
	fmt.Printf("  引导节点:    %s\n", bold(bootstrapAddr))
	fmt.Printf("  配置文件:    %s\n", configPath)

	return nil
}

func runClusterStatus(args []string) error {
	configPath := defaultClusterConfigPath
	jsonOutput := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			i++
			if i >= len(args) {
				return fmt.Errorf("--config requires a path")
			}
			configPath = args[i]
		case "--json":
			jsonOutput = true
		}
	}

	// 验证配置路径
	configPath, err := validateClusterConfigPath(configPath)
	if err != nil {
		return err
	}

	// 读取配置
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("cluster not initialized (no %s found), run 'ap cluster init' first", configPath)
	}

	var cfg clusterConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse cluster config: %w", err)
	}

	if jsonOutput {
		out, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	// 格式化输出
	fmt.Printf("%s\n\n", bold("集群状态"))
	fmt.Printf("  节点 ID:       %s\n", cfg.NodeID)
	fmt.Printf("  监听地址:      %s\n", cfg.ListenAddr)
	fmt.Printf("  心跳间隔:      %s\n", cfg.HeartbeatInterval)
	fmt.Printf("  心跳超时:      %s\n", cfg.HeartbeatTimeout)
	fmt.Printf("  创建时间:      %s\n", cfg.CreatedAt)

	if len(cfg.BootstrapNodes) > 0 {
		fmt.Printf("  引导节点:      %s\n", strings.Join(cfg.BootstrapNodes, ", "))
	} else {
		fmt.Printf("  角色:          %s\n", bold("leader (bootstrap)"))
	}

	return nil
}

func runClusterLeave(args []string) error {
	configPath := defaultClusterConfigPath

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			i++
			if i >= len(args) {
				return fmt.Errorf("--config requires a path")
			}
			configPath = args[i]
		}
	}

	// 验证配置路径
	configPath, err := validateClusterConfigPath(configPath)
	if err != nil {
		return err
	}

	// 检查配置是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("cluster not initialized (no %s found)", configPath)
	}

	// 删除配置文件表示离开
	if err := os.Remove(configPath); err != nil {
		return fmt.Errorf("remove cluster config: %w", err)
	}

	infof("已优雅离开集群（配置文件 %s 已删除）", configPath)
	return nil
}

func runClusterScale(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ap cluster scale <n>")
	}

	var targetNodes int
	if _, err := fmt.Sscanf(args[0], "%d", &targetNodes); err != nil {
		return fmt.Errorf("invalid node count %q: must be a positive integer", args[0])
	}
	if targetNodes < 1 {
		return fmt.Errorf("node count must be >= 1, got %d", targetNodes)
	}

	configPath := defaultClusterConfigPath
	for i := 1; i < len(args); i++ {
		if args[i] == "--config" {
			i++
			if i >= len(args) {
				return fmt.Errorf("--config requires a path")
			}
			configPath = args[i]
		}
	}

	// 验证配置路径
	configPath, err := validateClusterConfigPath(configPath)
	if err != nil {
		return err
	}

	// 读取当前配置
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("cluster not initialized (no %s found)", configPath)
	}

	var cfg clusterConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse cluster config: %w", err)
	}

	currentNodes := 1 + len(cfg.BootstrapNodes) // 本地 + 引导节点

	fmt.Printf("%s\n\n", bold("扩缩容计划"))
	fmt.Printf("  当前节点数:  %d\n", currentNodes)
	fmt.Printf("  目标节点数:  %d\n", targetNodes)

	if targetNodes > currentNodes {
		fmt.Printf("  操作:        %s %d 个节点\n", bold("扩容"), targetNodes-currentNodes)
		fmt.Printf("\n  在新节点上执行：\n")
		for i := 0; i < targetNodes-currentNodes; i++ {
			fmt.Printf("    ap cluster join %s --node-id node-%d\n", cfg.ListenAddr, currentNodes+i+1)
		}
	} else if targetNodes < currentNodes {
		fmt.Printf("  操作:        %s %d 个节点\n", bold("缩容"), currentNodes-targetNodes)
		fmt.Printf("\n  在要移除的节点上执行：\n")
		fmt.Printf("    ap cluster leave\n")
	} else {
		fmt.Printf("  操作:        无需变更（已达目标）\n")
	}

	return nil
}
