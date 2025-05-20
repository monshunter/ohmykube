package cluster

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

// NodeStatus represents node status
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

// NodeInfo stores detailed node information
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

// GenerateSSHCommand generates SSH command string
func (n *NodeInfo) GenerateSSHCommand() {
	n.SSHCommand = fmt.Sprintf("ssh -p %s %s@%s", n.SSHPort, n.SSHUser, n.ExtraInfo.IP)
}

// Cluster stores cluster information
type Cluster struct {
	ApiVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Name       string     `yaml:"name"`
	K8sVersion string     `yaml:"k8sVersion"`
	Launcher   string     `yaml:"launcher"`
	ProxyMode  string     `yaml:"proxyMode"`
	Master     NodeInfo   `yaml:"master"`
	Workers    []NodeInfo `yaml:"workers"`
}

func NewCluster(config *Config, master NodeInfo, workers []NodeInfo) *Cluster {
	return &Cluster{
		ApiVersion: ApiVersion,
		Kind:       KindCluster,
		Name:       config.Name,
		K8sVersion: config.KubernetesVersion,
		Launcher:   config.LauncherType,
		ProxyMode:  config.ProxyMode,
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

// SaveClusterInfomation saves cluster information to a file
func SaveClusterInfomation(info *Cluster) error {
	// Create the .ohmykube directory
	clusterDir, err := ClusterDir(info.Name)
	if err != nil {
		return fmt.Errorf("failed to get cluster directory: %w", err)
	}
	if err := os.MkdirAll(clusterDir, 0755); err != nil {
		return fmt.Errorf("failed to create .ohmykube directory: %w", err)
	}

	// Save cluster information to YAML file
	clusterYaml := filepath.Join(clusterDir, "cluster.yaml")
	data, err := yaml.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to serialize cluster information: %w", err)
	}

	if err := os.WriteFile(clusterYaml, data, 0644); err != nil {
		return fmt.Errorf("failed to save cluster information to %s: %w", clusterYaml, err)
	}

	return nil
}

// LoadClusterInfomation loads cluster information from a file
func LoadClusterInfomation(name string) (*Cluster, error) {
	clusterDir, err := ClusterDir(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster directory: %w", err)
	}

	clusterYaml := filepath.Join(clusterDir, "cluster.yaml")
	data, err := os.ReadFile(clusterYaml)
	if err != nil {
		return nil, fmt.Errorf("failed to read cluster information file: %w", err)
	}

	var info Cluster
	if err := yaml.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse cluster information: %w", err)
	}

	return &info, nil
}

func CheckClusterInfomationExists(name string) bool {
	clusterDir, err := ClusterDir(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(clusterDir, "cluster.yaml"))
	return err == nil
}

func RemoveCluster(name string) error {
	clusterDir, err := ClusterDir(name)
	if err != nil {
		return fmt.Errorf("failed to get cluster directory: %w", err)
	}
	return os.RemoveAll(clusterDir)
}

func OhMyKubeDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".ohmykube"), nil
}

func ClusterDir(name string) (string, error) {
	ohmykubeDir, err := OhMyKubeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(ohmykubeDir, name), nil
}
