package cluster

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// NodeStatus 表示节点状态
type NodeStatus string

const (
	NodeStatusRunning NodeStatus = "Running"
	NodeStatusStopped NodeStatus = "Stopped"
	NodeStatusUnknown NodeStatus = "Unknown"
)

// NodeInfo 保存节点详细信息
type NodeInfo struct {
	Name       string     `yaml:"name"`
	Role       string     `yaml:"role"`
	Status     NodeStatus `yaml:"status"`
	IP         string     `yaml:"ip"`
	CPU        int        `yaml:"cpu"`
	Memory     int        `yaml:"memory"`
	Disk       int        `yaml:"disk"`
	SSHPort    string     `yaml:"ssh_port"`
	SSHUser    string     `yaml:"ssh_user"`
	SSHCommand string     `yaml:"ssh_command,omitempty"`
}

// ClusterInfo 保存集群信息
type ClusterInfo struct {
	Name       string     `yaml:"name"`
	K8sVersion string     `yaml:"k8s_version"`
	Master     NodeInfo   `yaml:"master"`
	Workers    []NodeInfo `yaml:"workers"`
}

// SaveClusterInfo 保存集群信息到文件
func SaveClusterInfo(info *ClusterInfo) error {
	// 创建 .ohmykube 目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户主目录失败: %w", err)
	}

	ohmykubeDir := filepath.Join(homeDir, ".ohmykube")
	if err := os.MkdirAll(ohmykubeDir, 0755); err != nil {
		return fmt.Errorf("创建 .ohmykube 目录失败: %w", err)
	}

	// 将集群信息保存到 YAML 文件
	clusterYaml := filepath.Join(ohmykubeDir, "cluster.yaml")
	data, err := yaml.Marshal(info)
	if err != nil {
		return fmt.Errorf("序列化集群信息失败: %w", err)
	}

	if err := os.WriteFile(clusterYaml, data, 0644); err != nil {
		return fmt.Errorf("保存集群信息到 %s 失败: %w", clusterYaml, err)
	}

	return nil
}

// LoadClusterInfo 从文件加载集群信息
func LoadClusterInfo() (*ClusterInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户主目录失败: %w", err)
	}

	clusterYaml := filepath.Join(homeDir, ".ohmykube", "cluster.yaml")
	data, err := os.ReadFile(clusterYaml)
	if err != nil {
		return nil, fmt.Errorf("读取集群信息文件失败: %w", err)
	}

	var info ClusterInfo
	if err := yaml.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("解析集群信息失败: %w", err)
	}

	return &info, nil
}

// GenerateSSHCommand 生成SSH命令字符串
func (n *NodeInfo) GenerateSSHCommand() {
	n.SSHCommand = fmt.Sprintf("ssh -p %s %s@%s", n.SSHPort, n.SSHUser, n.IP)
}
