package main

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	agentv1 "agentprimordia/operator/api/v1"
	"agentprimordia/operator/controller"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(agentv1.AddToScheme(scheme))
}

func main() {
	metricsAddr := flag.String("metrics-addr", ":8080", "Metrics 服务器监听地址")
	enableLeaderElection := flag.Bool("leader-elect", false, "启用 Leader 选举")
	flag.Parse()

	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取 K8s 配置失败: %v\n", err)
		fmt.Fprintln(os.Stderr, "请确保 kubeconfig 已配置或运行在集群内")
		os.Exit(1)
	}

	mgr, err := manager.New(cfg, manager.Options{
		Scheme:             scheme,
		MetricsBindAddress: *metricsAddr,
		LeaderElection:     *enableLeaderElection,
		LeaderElectionID:   "agentprimordia-operator",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建 Manager 失败: %v\n", err)
		os.Exit(1)
	}

	// 注册 Controller
	if err := (&controller.AgentDeploymentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		fmt.Fprintf(os.Stderr, "注册 Controller 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("AgentPrimordia Operator 启动...")
	fmt.Printf("Metrics: %s\n", *metricsAddr)

	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		fmt.Fprintf(os.Stderr, "Operator 运行失败: %v\n", err)
		os.Exit(1)
	}
}
