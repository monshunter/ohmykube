package lb

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"

	"github.com/monshunter/ohmykube/pkg/multipass"
)

// MetalLBInstaller 负责安装 MetalLB LoadBalancer
type MetalLBInstaller struct {
	MultipassClient *multipass.Client
	MasterNode      string
	Version         string
}

// NewMetalLBInstaller 创建 MetalLB 安装器
func NewMetalLBInstaller(mpClient *multipass.Client, masterNode string) *MetalLBInstaller {
	return &MetalLBInstaller{
		MultipassClient: mpClient,
		MasterNode:      masterNode,
		Version:         "v0.13.7", // MetalLB 版本
	}
}

// Install 安装 MetalLB
func (m *MetalLBInstaller) Install() error {
	// 安装 MetalLB
	installCmd := fmt.Sprintf(`
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/%s/config/manifests/metallb-native.yaml
`, m.Version)

	_, err := m.MultipassClient.ExecCommand(m.MasterNode, installCmd)
	if err != nil {
		return fmt.Errorf("安装 MetalLB 失败: %w", err)
	}

	// 等待 MetalLB 控制器就绪
	_, err = m.MultipassClient.ExecCommand(m.MasterNode, `
kubectl wait --namespace metallb-system \
  --for=condition=ready pod \
  --selector=app=metallb \
  --timeout=3m
`)
	if err != nil {
		return fmt.Errorf("等待 MetalLB 就绪超时: %w", err)
	}

	// 获取虚拟机的 IP 地址范围用于 MetalLB 地址池
	ipRange, err := m.getAddressPoolRange()
	if err != nil {
		return fmt.Errorf("获取 IP 地址范围失败: %w", err)
	}

	// 创建 MetalLB 地址池和 L2 通告配置
	configMap := fmt.Sprintf(`
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: first-pool
  namespace: metallb-system
spec:
  addresses:
  - %s
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: l2-advert
  namespace: metallb-system
spec:
  ipAddressPools:
  - first-pool
`, ipRange)

	// 创建临时配置文件
	tmpfile, err := ioutil.TempFile("", "metallb-config-*.yaml")
	if err != nil {
		return fmt.Errorf("创建临时 MetalLB 配置文件失败: %w", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(configMap)); err != nil {
		return fmt.Errorf("写入 MetalLB 配置文件失败: %w", err)
	}
	if err := tmpfile.Close(); err != nil {
		return fmt.Errorf("关闭 MetalLB 配置文件失败: %w", err)
	}

	// 将配置文件传输到虚拟机
	remoteConfigPath := "/tmp/metallb-config.yaml"
	if err := m.MultipassClient.TransferFile(tmpfile.Name(), m.MasterNode, remoteConfigPath); err != nil {
		return fmt.Errorf("传输 MetalLB 配置文件到虚拟机失败: %w", err)
	}

	// 应用配置
	_, err = m.MultipassClient.ExecCommand(m.MasterNode, fmt.Sprintf("kubectl apply -f %s", remoteConfigPath))
	if err != nil {
		return fmt.Errorf("应用 MetalLB 配置失败: %w", err)
	}

	return nil
}

// getAddressPoolRange 获取适合 MetalLB 的 IP 地址范围
func (m *MetalLBInstaller) getAddressPoolRange() (string, error) {
	// 获取主节点 IP 地址
	output, err := m.MultipassClient.ExecCommand(m.MasterNode, "ip -4 addr show | grep inet")
	if err != nil {
		return "", fmt.Errorf("无法获取节点 IP 地址: %w", err)
	}

	// 解析 IP 地址
	re := regexp.MustCompile(`inet (\d+\.\d+\.\d+\.)(\d+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) < 3 {
		return "", fmt.Errorf("无法解析 IP 地址格式")
	}

	// 使用相同子网最后 10 个地址
	prefix := matches[1]
	base, _ := strconv.Atoi(matches[2])
	start := base + 100
	end := start + 10

	return fmt.Sprintf("%s%d-%s%d", prefix, start, prefix, end), nil
}
