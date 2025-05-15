package kubeadm

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/monshunter/ohmykube/pkg/default/kubeadm"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// KubeadmConfig 保存 kubeadm 配置
type KubeadmConfig struct {
	SSHClient         *ssh.Client
	MasterNode        string
	PodCIDR           string
	ServiceCIDR       string
	KubernetesVersion string
	CustomConfigPath  string // 新增：自定义配置路径
}

// NewKubeadmConfig 创建一个新的 kubeadm 配置
func NewKubeadmConfig(sshClient *ssh.Client, k8sVersion string, masterNode string) *KubeadmConfig {
	return &KubeadmConfig{
		SSHClient:         sshClient,
		MasterNode:        masterNode,
		PodCIDR:           "10.244.0.0/16",
		ServiceCIDR:       "10.96.0.0/12",
		KubernetesVersion: k8sVersion,
	}
}

// InitMaster 初始化 Kubernetes 控制平面
func (k *KubeadmConfig) InitMaster() error {
	// 使用新的配置生成系统
	configPath, err := kubeadm.GenerateKubeadmConfig(
		k.KubernetesVersion,
		k.PodCIDR,
		k.ServiceCIDR,
		k.CustomConfigPath,
	)
	if err != nil {
		return fmt.Errorf("生成 kubeadm 配置失败: %w", err)
	}
	defer os.Remove(configPath) // 确保临时文件被删除

	// 将配置文件传输到虚拟机
	remoteConfigPath := "/tmp/kubeadm-config.yaml"
	if err := k.SSHClient.TransferFile(configPath, remoteConfigPath); err != nil {
		return fmt.Errorf("传输 kubeadm 配置文件到虚拟机失败: %w", err)
	}

	// 执行 kubeadm init
	initCmd := fmt.Sprintf("sudo kubeadm init --config %s --upload-certs", remoteConfigPath)
	_, err = k.SSHClient.RunCommand(initCmd)
	if err != nil {
		return fmt.Errorf("执行 kubeadm init 失败: %w", err)
	}
	// 配置 kubectl
	configCmd := `mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config`
	_, err = k.SSHClient.RunCommand(configCmd)
	if err != nil {
		return fmt.Errorf("配置 kubectl 失败: %w", err)
	}

	return nil
}

// JoinNode 将节点加入集群
func (k *KubeadmConfig) JoinNode(nodeName, joinCommand string) error {
	_, err := k.SSHClient.RunCommand("sudo " + joinCommand)
	if err != nil {
		return fmt.Errorf("节点加入集群失败: %w", err)
	}
	return nil
}

// GetKubeconfig 获取 kubeconfig 到本地
func (k *KubeadmConfig) GetKubeconfig() (string, error) {
	// 获取 kubeconfig 内容
	output, err := k.SSHClient.RunCommand("cat $HOME/.kube/config")
	if err != nil {
		return "", fmt.Errorf("获取 kubeconfig 失败: %w", err)
	}

	// 创建本地 kubeconfig 文件
	kubeDir := filepath.Join(os.Getenv("HOME"), ".kube")
	if err := os.MkdirAll(kubeDir, 0755); err != nil {
		return "", fmt.Errorf("创建 .kube 目录失败: %w", err)
	}

	ohmykubeConfig := filepath.Join(kubeDir, "ohmykube-config")
	if err := os.WriteFile(ohmykubeConfig, []byte(output), 0644); err != nil {
		return "", fmt.Errorf("写入 kubeconfig 文件失败: %w", err)
	}

	return ohmykubeConfig, nil
}

// SetCustomConfig 设置自定义配置文件路径
func (k *KubeadmConfig) SetCustomConfig(configPath string) {
	k.CustomConfigPath = configPath
}
