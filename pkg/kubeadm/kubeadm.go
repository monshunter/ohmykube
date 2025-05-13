package kubeadm

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/monshunter/ohmykube/pkg/multipass"
)

// KubeadmConfig 保存 kubeadm 配置
type KubeadmConfig struct {
	MultipassClient   *multipass.Client
	MasterNode        string
	PodCIDR           string
	ServiceCIDR       string
	KubernetesVersion string
}

// NewKubeadmConfig 创建一个新的 kubeadm 配置
func NewKubeadmConfig(mpClient *multipass.Client, k8sVersion string, masterNode string) *KubeadmConfig {
	return &KubeadmConfig{
		MultipassClient:   mpClient,
		MasterNode:        masterNode,
		PodCIDR:           "10.244.0.0/16",
		ServiceCIDR:       "10.96.0.0/12",
		KubernetesVersion: k8sVersion,
	}
}

// InitMaster 初始化 Kubernetes 控制平面
func (k *KubeadmConfig) InitMaster() (string, error) {
	// 准备 kubeadm 配置文件
	kubeadmConfig := fmt.Sprintf(`apiVersion: kubeadm.k8s.io/v1beta3
kind: InitConfiguration
nodeRegistration:
  criSocket: unix:///var/run/containerd/containerd.sock
---
apiVersion: kubeadm.k8s.io/v1beta3
kind: ClusterConfiguration
kubernetesVersion: v%s
networking:
  podSubnet: %s
  serviceSubnet: %s
---
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
cgroupDriver: systemd
`, k.KubernetesVersion, k.PodCIDR, k.ServiceCIDR)

	// 创建临时文件
	tmpfile, err := os.CreateTemp("", "kubeadm-config-*.yaml")
	if err != nil {
		return "", fmt.Errorf("创建临时 kubeadm 配置文件失败: %w", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(kubeadmConfig)); err != nil {
		return "", fmt.Errorf("写入 kubeadm 配置文件失败: %w", err)
	}
	if err := tmpfile.Close(); err != nil {
		return "", fmt.Errorf("关闭 kubeadm 配置文件失败: %w", err)
	}

	// 将配置文件传输到虚拟机
	remoteConfigPath := "/tmp/kubeadm-config.yaml"
	if err := k.MultipassClient.TransferFile(tmpfile.Name(), k.MasterNode, remoteConfigPath); err != nil {
		return "", fmt.Errorf("传输 kubeadm 配置文件到虚拟机失败: %w", err)
	}

	// 执行 kubeadm init
	initCmd := fmt.Sprintf("sudo kubeadm init --config %s --upload-certs", remoteConfigPath)
	output, err := k.MultipassClient.ExecCommand(k.MasterNode, initCmd)
	if err != nil {
		return "", fmt.Errorf("执行 kubeadm init 失败: %w", err)
	}

	// 从输出中提取 join 命令
	joinCommand := extractJoinCommand(output)
	if joinCommand == "" {
		return "", fmt.Errorf("无法从 kubeadm init 输出中提取 join 命令")
	}

	// 配置 kubectl
	configCmd := `mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config`
	_, err = k.MultipassClient.ExecCommand(k.MasterNode, configCmd)
	if err != nil {
		return "", fmt.Errorf("配置 kubectl 失败: %w", err)
	}

	return joinCommand, nil
}

// JoinNode 将节点加入集群
func (k *KubeadmConfig) JoinNode(nodeName, joinCommand string) error {
	_, err := k.MultipassClient.ExecCommand(nodeName, "sudo "+joinCommand)
	if err != nil {
		return fmt.Errorf("节点加入集群失败: %w", err)
	}
	return nil
}

// GetKubeconfig 获取 kubeconfig 到本地
func (k *KubeadmConfig) GetKubeconfig() (string, error) {
	// 获取 kubeconfig 内容
	output, err := k.MultipassClient.ExecCommand(k.MasterNode, "cat $HOME/.kube/config")
	if err != nil {
		return "", fmt.Errorf("获取 kubeconfig 失败: %w", err)
	}

	// 创建本地 kubeconfig 文件
	kubeDir := filepath.Join(os.Getenv("HOME"), ".kube")
	if err := os.MkdirAll(kubeDir, 0755); err != nil {
		return "", fmt.Errorf("创建 .kube 目录失败: %w", err)
	}

	ohmykubeConfig := filepath.Join(kubeDir, "ohmykube-config")
	if err := ioutil.WriteFile(ohmykubeConfig, []byte(output), 0644); err != nil {
		return "", fmt.Errorf("写入 kubeconfig 文件失败: %w", err)
	}

	return ohmykubeConfig, nil
}

// extractJoinCommand 从 kubeadm init 输出中提取 join 命令
func extractJoinCommand(output string) string {
	lines := strings.Split(output, "\n")
	var joinCommand string
	var collectingJoin bool

	for _, line := range lines {
		if strings.Contains(line, "kubeadm join") {
			joinCommand = strings.TrimSpace(line)
			collectingJoin = true
			continue
		}

		if collectingJoin {
			if strings.HasPrefix(strings.TrimSpace(line), "--") {
				joinCommand += " " + strings.TrimSpace(line)
			} else if joinCommand != "" {
				break
			}
		}
	}

	return joinCommand
}
