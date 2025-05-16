package lb

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/default/metallb"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// MetalLBInstaller 用于安装和配置MetalLB负载均衡器
type MetalLBInstaller struct {
	sshClient *ssh.Client
	nodeName  string
}

// NewMetalLBInstaller 创建一个新的MetalLB安装器
func NewMetalLBInstaller(sshClient *ssh.Client, nodeName string) *MetalLBInstaller {
	return &MetalLBInstaller{
		sshClient: sshClient,
		nodeName:  nodeName,
	}
}

// Install 安装MetalLB负载均衡器
func (m *MetalLBInstaller) Install() error {
	// 安装MetalLB
	metallbCmd := `
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.9/config/manifests/metallb-native.yaml
`
	_, err := m.sshClient.RunCommand(metallbCmd)
	if err != nil {
		return fmt.Errorf("安装 MetalLB 失败: %w", err)
	}

	// 等待MetalLB部署完成
	waitCmd := `
kubectl wait --namespace metallb-system --for=condition=ready pod --selector=app=metallb --timeout=90s
`
	_, err = m.sshClient.RunCommand(waitCmd)
	if err != nil {
		return fmt.Errorf("等待 MetalLB 部署完成失败: %w", err)
	}

	// 获取节点IP地址范围
	ipRange, err := m.getMetalLBAddressRange()
	if err != nil {
		return fmt.Errorf("获取 MetalLB 地址范围失败: %w", err)
	}

	// 导入配置模板
	configYAML := strings.Replace(metallb.CONFIG_YAML, "# - 192.168.64.200 - 192.168.64.250", fmt.Sprintf("- %s", ipRange), 1)

	// 创建配置文件
	configFile := "/tmp/metallb-config.yaml"
	createConfigCmd := fmt.Sprintf("cat <<EOF | tee %s\n%sEOF", configFile, configYAML)
	_, err = m.sshClient.RunCommand(createConfigCmd)
	if err != nil {
		return fmt.Errorf("创建 MetalLB 配置文件失败: %w", err)
	}

	// 应用配置
	applyConfigCmd := fmt.Sprintf("kubectl apply -f %s", configFile)
	_, err = m.sshClient.RunCommand(applyConfigCmd)
	if err != nil {
		return fmt.Errorf("应用 MetalLB 配置失败: %w", err)
	}

	return nil
}

// getMetalLBAddressRange 获取适合MetalLB的IP地址范围
func (m *MetalLBInstaller) getMetalLBAddressRange() (string, error) {
	// 获取主节点IP地址
	ipCmd := "ip -4 addr show | grep inet | grep -v '127.0.0.1' | head -1 | awk '{print $2}' | cut -d/ -f1"
	output, err := m.sshClient.RunCommand(ipCmd)
	if err != nil {
		return "", fmt.Errorf("获取节点IP地址失败: %w", err)
	}

	// 解析IP地址
	ip := strings.TrimSpace(output)
	ipParts := strings.Split(ip, ".")
	if len(ipParts) != 4 {
		return "", fmt.Errorf("IP地址格式错误: %s", ip)
	}

	// 使用相同子网的一段IP地址作为LoadBalancer IP池
	// 例如: 192.168.64.100 -> 192.168.64.200-192.168.64.250
	prefix := strings.Join(ipParts[:3], ".")
	startIP := 200
	endIP := 250

	return fmt.Sprintf("%s.%d - %s.%d", prefix, startIP, prefix, endIP), nil
}
