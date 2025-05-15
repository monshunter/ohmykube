package clusterinfo

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/ssh"
)

// ClusterInfo 提供获取集群信息的功能
type ClusterInfo struct {
	SSHClient *ssh.Client
}

// NewClusterInfo 创建新的集群信息获取器
func NewClusterInfo(sshClient *ssh.Client) *ClusterInfo {
	return &ClusterInfo{
		SSHClient: sshClient,
	}
}

// GetPodCIDR 获取集群Pod CIDR
func (c *ClusterInfo) GetPodCIDR() (string, error) {
	// 默认值
	defaultPodCIDR := "10.244.0.0/16"

	// 首先尝试从kubeadm-config获取
	getPodCIDRFromConfigMap := `
kubectl -n kube-system get cm kubeadm-config -o jsonpath='{.data.ClusterConfiguration}' | grep -A3 networking | grep podSubnet | awk '{print $2}'
`
	output, err := c.SSHClient.RunCommand(getPodCIDRFromConfigMap)
	if err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	// 如果失败，尝试从kube-controller-manager的启动参数中获取
	getFromControllerManager := `
kubectl -n kube-system get pod -l component=kube-controller-manager -o jsonpath='{.items[0].spec.containers[0].command}' | grep -o -- '--cluster-cidr=[0-9./]*' | cut -d= -f2
`
	output, err = c.SSHClient.RunCommand(getFromControllerManager)
	if err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	// 两种方法都失败，返回默认值并添加警告
	fmt.Printf("警告: 无法获取集群Pod CIDR，使用默认值 %s\n", defaultPodCIDR)
	return defaultPodCIDR, nil
}

// GetServiceCIDR 获取集群Service CIDR
func (c *ClusterInfo) GetServiceCIDR() (string, error) {
	// 默认值
	defaultServiceCIDR := "10.96.0.0/12"

	// 首先尝试从kubeadm-config获取
	getServiceCIDRFromConfigMap := `
kubectl -n kube-system get cm kubeadm-config -o jsonpath='{.data.ClusterConfiguration}' | grep -A5 networking | grep serviceSubnet | awk '{print $2}'
`
	output, err := c.SSHClient.RunCommand(getServiceCIDRFromConfigMap)
	if err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	// 如果失败，尝试从kube-apiserver的启动参数中获取
	getFromAPIServer := `
kubectl -n kube-system get pod -l component=kube-apiserver -o jsonpath='{.items[0].spec.containers[0].command}' | grep -o -- '--service-cluster-ip-range=[0-9./]*' | cut -d= -f2
`
	output, err = c.SSHClient.RunCommand(getFromAPIServer)
	if err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	// 两种方法都失败，返回默认值并添加警告
	fmt.Printf("警告: 无法获取集群Service CIDR，使用默认值 %s\n", defaultServiceCIDR)
	return defaultServiceCIDR, nil
}

// GetKubernetesVersion 获取集群Kubernetes版本
func (c *ClusterInfo) GetKubernetesVersion() (string, error) {
	cmd := "kubectl version -o json | jq -r '.serverVersion.gitVersion'"
	output, err := c.SSHClient.RunCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("获取Kubernetes版本失败: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// GetClusterDNSDomain 获取集群DNS域名
func (c *ClusterInfo) GetClusterDNSDomain() (string, error) {
	// 默认值
	defaultDNSDomain := "cluster.local"

	// 从kubeadm-config获取
	getDNSDomainFromConfigMap := `
kubectl -n kube-system get cm kubeadm-config -o jsonpath='{.data.ClusterConfiguration}' | grep -A5 networking | grep dnsDomain | awk '{print $2}'
`
	output, err := c.SSHClient.RunCommand(getDNSDomainFromConfigMap)
	if err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	// 如果失败，返回默认值
	fmt.Printf("警告: 无法获取集群DNS域名，使用默认值 %s\n", defaultDNSDomain)
	return defaultDNSDomain, nil
}

// GetNodeCount 获取集群节点数
func (c *ClusterInfo) GetNodeCount() (int, error) {
	cmd := "kubectl get nodes --no-headers | wc -l"
	output, err := c.SSHClient.RunCommand(cmd)
	if err != nil {
		return 0, fmt.Errorf("获取集群节点数失败: %w", err)
	}

	output = strings.TrimSpace(output)
	// 尝试将输出转换为整数
	var count int
	_, err = fmt.Sscanf(output, "%d", &count)
	if err != nil {
		return 0, fmt.Errorf("无法解析节点数: %w", err)
	}

	return count, nil
}
