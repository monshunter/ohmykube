package cluster

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

// NodeStatus 表示节点状态
type NodeStatus string

const (
	NodeStatusRunning NodeStatus = "Running"
	NodeStatusStopped NodeStatus = "Stopped"
	NodeStatusUnknown NodeStatus = "Unknown"
)

const (
	KindCluster = "Cluster"
	ApiVersion  = "ohmykube.dev/v1alpha1"
)

// NodeInfo 保存节点详细信息
type NodeInfo struct {
	Name       string        `yaml:"name"`
	Role       string        `yaml:"role"`
	CPU        int           `yaml:"cpu"`
	Memory     int           `yaml:"memory"`
	Disk       int           `yaml:"disk"`
	SSHPort    string        `yaml:"sshPort"`
	SSHUser    string        `yaml:"sshUser"`
	SSHCommand string        `yaml:"sshCommand,omitempty"`
	ExtraInfo  NodeExtraInfo `yaml:"extraInfo,omitempty"`
}

type NodeExtraInfo struct {
	Name     string     `json:"name"`
	Hostname string     `json:"hostname"`
	IP       string     `json:"ip"`
	Status   NodeStatus `json:"status"`
	Release  string     `json:"release"`
	Kernel   string     `json:"kernel"`
	Arch     string     `json:"arch"`
	OS       string     `json:"os"`
}

func NewNodeInfo(name string, role string, cpu int, memory int, disk int) NodeInfo {
	return NodeInfo{
		Name:    name,
		Role:    role,
		CPU:     cpu,
		Memory:  memory,
		Disk:    disk,
		SSHPort: "22",
		SSHUser: "root",
	}
}

// Cluster 保存集群信息
type Cluster struct {
	ApiVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Name       string     `yaml:"name"`
	K8sVersion string     `yaml:"k8sVersion"`
	Master     NodeInfo   `yaml:"master"`
	Workers    []NodeInfo `yaml:"workers"`
}

func NewCluster(name string, k8sVersion string, master NodeInfo, workers []NodeInfo) *Cluster {
	return &Cluster{
		ApiVersion: ApiVersion,
		Kind:       KindCluster,
		Name:       name,
		K8sVersion: k8sVersion,
		Master:     master,
		Workers:    workers,
	}
}

func (c *Cluster) GetMasterIP() string {
	return c.Master.ExtraInfo.IP
}

func (c *Cluster) Nodes2IPsMap() map[string]string {
	ips := make(map[string]string)
	ips[c.Master.Name] = c.Master.ExtraInfo.IP
	for _, node := range c.Workers {
		ips[node.Name] = node.ExtraInfo.IP
	}
	return ips
}

func (c *Cluster) GetNodeByIP(ip string) *NodeInfo {
	for _, node := range c.Workers {
		if node.ExtraInfo.IP == ip {
			return &node
		}
	}
	return nil
}

func (c *Cluster) GetNodeByName(name string) *NodeInfo {
	if c.Master.Name == name {
		return &c.Master
	}
	for _, node := range c.Workers {
		if node.Name == name {
			return &node
		}
	}
	return nil
}

func (c *Cluster) RemoveNode(name string) {
	if c.Master.Name == name {
		c.Master = NodeInfo{}
	}
	for i, node := range c.Workers {
		if node.Name == name {
			c.Workers = slices.Delete(c.Workers, i, i+1)
			break
		}
	}
}

func (c *Cluster) UpdateWithExtraInfo(extra []NodeExtraInfo) {
	for _, node := range extra {
		if node.Name == c.Master.Name {
			c.Master.ExtraInfo = node
			c.Master.GenerateSSHCommand()
			continue
		}
		for i, worker := range c.Workers {
			if worker.Name == node.Name {
				c.Workers[i].ExtraInfo = node
				c.Workers[i].GenerateSSHCommand()
				break
			}
		}
	}
}

func (c *Cluster) AddNode(node NodeInfo) {
	c.Workers = append(c.Workers, node)
}

// SaveCluster 保存集群信息到文件
func SaveClusterInfomation(info *Cluster) error {
	// 创建 .ohmykube 目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户主目录失败: %w", err)
	}

	ohmykubeDir := filepath.Join(homeDir, ".ohmykube", info.Name)
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

// LoadCluster 从文件加载集群信息
func LoadClusterInfomation(name string) (*Cluster, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户主目录失败: %w", err)
	}

	clusterYaml := filepath.Join(homeDir, ".ohmykube", name, "cluster.yaml")
	data, err := os.ReadFile(clusterYaml)
	if err != nil {
		return nil, fmt.Errorf("读取集群信息文件失败: %w", err)
	}

	var info Cluster
	if err := yaml.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("解析集群信息失败: %w", err)
	}

	return &info, nil
}

// GenerateSSHCommand 生成SSH命令字符串
func (n *NodeInfo) GenerateSSHCommand() {
	n.SSHCommand = fmt.Sprintf("ssh -p %s %s@%s", n.SSHPort, n.SSHUser, n.ExtraInfo.IP)
}
