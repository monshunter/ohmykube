package environment

import (
	"os"
)

// SSHCommandRunner 定义了执行SSH命令的接口
type SSHCommandRunner interface {
	RunSSHCommand(nodeName string, command string) (string, error)
}

// InitOptions 定义环境初始化选项
type InitOptions struct {
	DisableSwap      bool   // 是否禁用swap
	EnableIPVS       bool   // 是否启用IPVS模式
	ContainerRuntime string // 容器运行时，默认为containerd
	K8sMirrorURL     string // Kubernetes源地址，默认为官方源
}

// DefaultInitOptions 返回默认初始化选项
func DefaultInitOptions() InitOptions {
	// 从环境变量读取K8s源地址配置，如果未设置则使用官方源
	k8sMirrorURL := os.Getenv("OHMYKUBE_K8S_MIRROR_URL")
	if k8sMirrorURL == "" {
		k8sMirrorURL = "https://pkgs.k8s.io/core:/stable:/v1.33/deb"
	}

	return InitOptions{
		DisableSwap:      true,         // 默认禁用swap
		EnableIPVS:       false,        // 默认不启用IPVS
		ContainerRuntime: "containerd", // 默认使用containerd
		K8sMirrorURL:     k8sMirrorURL, // 使用环境变量配置的源或默认源
	}
}

// EnvironmentInitializer 是环境初始化器的接口
type EnvironmentInitializer interface {
	// DisableSwap 禁用swap
	DisableSwap() error

	// EnableIPVS 启用IPVS模块
	EnableIPVS() error

	// InstallContainerd 安装和配置containerd
	InstallContainerd() error

	// InstallK8sComponents 安装kubeadm、kubectl、kubelet
	InstallK8sComponents() error

	// Initialize 执行所有初始化步骤
	Initialize() error
}

// NodeInitResult 表示单个节点的初始化结果
type NodeInitResult struct {
	NodeName string
	Success  bool
	Error    error
}

// BatchInitializer 是批量环境初始化器的接口，支持并行初始化多个节点
type BatchInitializer interface {
	// Initialize 并行初始化所有节点
	Initialize() error

	// InitializeWithConcurrencyLimit 使用并发限制的并行初始化
	InitializeWithConcurrencyLimit(maxConcurrency int) error

	// InitializeWithResults 并行初始化所有节点并返回详细结果
	InitializeWithResults() []NodeInitResult

	// InitializeWithConcurrencyLimitAndResults 使用并发限制的并行初始化并返回详细结果
	InitializeWithConcurrencyLimitAndResults(maxConcurrency int) []NodeInitResult
}
