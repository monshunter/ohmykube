package cni

import (
	"fmt"
	"os"

	"github.com/monshunter/ohmykube/pkg/ssh"
)

// CiliumInstaller 负责安装 Cilium CNI
type CiliumInstaller struct {
	SSHClient  *ssh.Client
	MasterNode string
	Version    string
}

// NewCiliumInstaller 创建 Cilium 安装器
func NewCiliumInstaller(sshClient *ssh.Client, masterNode string) *CiliumInstaller {
	return &CiliumInstaller{
		SSHClient:  sshClient,
		MasterNode: masterNode,
		Version:    "1.14.5", // Cilium 版本
	}
}

// Install 安装 Cilium CNI
func (c *CiliumInstaller) Install() error {
	// 准备 Cilium 配置
	ciliumConfig := `apiVersion: helm.k8s.io/v1
kind: HelmRelease
metadata:
  name: cilium
  namespace: kube-system
spec:
  chart:
    repository: https://helm.cilium.io
    name: cilium
    version: %s
  values:
    ipam:
      mode: kubernetes
    hubble:
      relay:
        enabled: true
      ui:
        enabled: true
    kubeProxyReplacement: true
    k8sServiceHost: 127.0.0.1
    k8sServicePort: 6443
`
	ciliumConfig = fmt.Sprintf(ciliumConfig, c.Version)

	// 创建临时文件
	tmpfile, err := os.CreateTemp("", "cilium-config-*.yaml")
	if err != nil {
		return fmt.Errorf("创建临时 Cilium 配置文件失败: %w", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(ciliumConfig)); err != nil {
		return fmt.Errorf("写入 Cilium 配置文件失败: %w", err)
	}
	if err := tmpfile.Close(); err != nil {
		return fmt.Errorf("关闭 Cilium 配置文件失败: %w", err)
	}

	// 将配置文件传输到虚拟机
	remoteConfigPath := "/tmp/cilium-config.yaml"
	if err := c.SSHClient.TransferFile(tmpfile.Name(), remoteConfigPath); err != nil {
		return fmt.Errorf("传输 Cilium 配置文件到虚拟机失败: %w", err)
	}

	// 安装 Helm (如果需要)
	_, err = c.SSHClient.RunCommand(`
if ! command -v helm &> /dev/null; then
  curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
fi
`)
	if err != nil {
		return fmt.Errorf("安装 Helm 失败: %w", err)
	}

	// 添加 Cilium Helm 仓库
	_, err = c.SSHClient.RunCommand(`
helm repo add cilium https://helm.cilium.io
helm repo update
`)
	if err != nil {
		return fmt.Errorf("添加 Cilium Helm 仓库失败: %w", err)
	}

	// 安装 Cilium
	installCmd := fmt.Sprintf(`
helm install cilium cilium/cilium --version %s \
  --namespace kube-system \
  --set ipam.mode=kubernetes \
  --set hubble.relay.enabled=true \
  --set hubble.ui.enabled=true \
  --set kubeProxyReplacement=true \
  --set k8sServiceHost=127.0.0.1 \
  --set k8sServicePort=6443
`, c.Version)

	_, err = c.SSHClient.RunCommand(installCmd)
	if err != nil {
		return fmt.Errorf("安装 Cilium 失败: %w", err)
	}

	// 等待 Cilium 启动完成
	_, err = c.SSHClient.RunCommand(`
kubectl -n kube-system wait --for=condition=ready pod -l k8s-app=cilium --timeout=5m
`)
	if err != nil {
		return fmt.Errorf("等待 Cilium 就绪超时: %w", err)
	}

	return nil
}
