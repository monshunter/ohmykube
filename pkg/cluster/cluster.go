package cluster

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var clusterLock sync.RWMutex

var localRand = rand.New(rand.NewSource(time.Now().UnixNano()))

type Phase string

const (
	PhasePending Phase = "Pending"
	PhaseRunning Phase = "Running"
	PhaseFailed  Phase = "Failed"
	PhaseUnknown Phase = "Unknown"
)

const (
	KindCluster = "Cluster"
	ApiVersion  = "ohmykube.dev/v1alpha1"
)

type Metadata struct {
	Name        string            `yaml:"name,omitempty"`
	Launcher    string            `yaml:"launcher,omitempty"`
	ProxyMode   string            `yaml:"proxyMode,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
	Taints      []Taint           `yaml:"taints,omitempty"`
}

type Taint struct {
	Key    string `yaml:"key,omitempty"`
	Value  string `yaml:"value,omitempty"`
	Effect string `yaml:"effect,omitempty"`
}

// Node stores detailed node information
type Node struct {
	Metadata `yaml:"metadata,omitempty"`
	Spec     NodeSpec   `yaml:"spec,omitempty"`
	Status   NodeStatus `yaml:"status,omitempty"`
}

type NodeSpec struct {
	Role   string `yaml:"role,omitempty"`
	CPU    int    `yaml:"cpu,omitempty"`
	Memory int    `yaml:"memory,omitempty"`
	Disk   int    `yaml:"disk,omitempty"`
}

type NodeStatus struct {
	Phase Phase `yaml:"phase,omitempty"`

	Hostname string `yaml:"hostname,omitempty"`
	// IP ipv4 address
	IP         string      `yaml:"ip,omitempty"`
	IPv6       string      `yaml:"ipv6,omitempty"`
	IPs        []string    `yaml:"ips,omitempty"`
	Release    string      `yaml:"release,omitempty"`
	Kernel     string      `yaml:"kernel,omitempty"`
	Arch       string      `yaml:"arch,omitempty"`
	OS         string      `yaml:"os,omitempty"`
	Ready      bool        `yaml:"ready,omitempty"`
	Conditions []Condition `yaml:"conditions,omitempty"`
}

type ConditionType string

const (
	ConditionTypeReady           ConditionType = "Ready"
	ConditionTypeNodeInitialized ConditionType = "NodeInitialized"
	ConditionTypeNodeReady       ConditionType = "NodeReady"
)

type ConditionStatus string

const (
	ConditionStatusTrue    ConditionStatus = "True"
	ConditionStatusFalse   ConditionStatus = "False"
	ConditionStatusUnknown ConditionStatus = "Unknown"
)

type Condition struct {
	Type               ConditionType   `yaml:"type,omitempty"`
	Status             ConditionStatus `yaml:"status,omitempty"`
	Reason             string          `yaml:"reason,omitempty"`
	Message            string          `yaml:"message,omitempty"`
	LastTransitionTime time.Time       `yaml:"lastTransitionTime,omitempty"`
}

func NewCondition(t ConditionType, s ConditionStatus, r string, m string) Condition {
	return Condition{
		Type:    t,
		Status:  s,
		Reason:  r,
		Message: m,
	}
}

func (c Condition) IsReady() bool {
	return c.Type == ConditionTypeReady && c.Status == ConditionStatusTrue
}

func (c Condition) IsFailed() bool {
	return c.Type == ConditionTypeReady && c.Status == ConditionStatusFalse
}

func (c Condition) IsUnknown() bool {
	return c.Type == ConditionTypeReady && c.Status == ConditionStatusUnknown
}

func (c Condition) IsPending() bool {
	return c.Status != ConditionStatusTrue
}

func NewNode(name string, role string, cpu int, memory int, disk int) *Node {
	return &Node{
		Metadata: Metadata{
			Name: name,
		},
		Spec: NodeSpec{
			Role:   role,
			CPU:    cpu,
			Memory: memory,
			Disk:   disk,
		},
	}
}

func (n *Node) UpdateStatus(status NodeStatus) {
	n.Status = status
}

type Auth struct {
	User    string `yaml:"user"`
	Port    string `yaml:"port"`
	KeyFile string `yaml:"keyFile"`
}

// Cluster stores cluster information
type Cluster struct {
	ApiVersion string  `yaml:"apiVersion"`
	Kind       string  `yaml:"kind"`
	Name       string  `yaml:"name"`
	K8sVersion string  `yaml:"k8sVersion"`
	Launcher   string  `yaml:"launcher"`
	ProxyMode  string  `yaml:"proxyMode"`
	Master     *Node   `yaml:"master"`
	Workers    []*Node `yaml:"workers"`
	Auth       Auth    `yaml:"auth"`
}

func NewCluster(config *Config) *Cluster {
	cls := &Cluster{
		ApiVersion: ApiVersion,
		Kind:       KindCluster,
		Name:       config.Name,
		K8sVersion: config.KubernetesVersion,
		ProxyMode:  config.ProxyMode,
		Launcher:   config.LauncherType,
		Master:     nil,
		Workers:    make([]*Node, len(config.Workers)),
		Auth:       Auth{},
	}
	cls.Master = NewNode(cls.GenMasterName(), RoleMaster, config.Master.CPU,
		config.Master.Memory, config.Master.Disk)
	for i := range config.Workers {
		cls.Workers[i] = NewNode(cls.GenWorkerName(), RoleWorker, config.Workers[i].CPU,
			config.Workers[i].Memory, config.Workers[i].Disk)
	}
	return cls
}

func (c *Cluster) GetMasterIP() string {
	return c.Master.Status.IP
}

func (c *Cluster) GetMasterName() string {
	return c.Master.Name
}

func (c *Cluster) Nodes2IPsMap() map[string]string {
	ips := make(map[string]string)
	ips[c.Master.Name] = c.Master.Status.IP
	for _, node := range c.Workers {
		ips[node.Name] = node.Status.IP
	}
	return ips
}

func (c *Cluster) GetNodeByIP(ip string) *Node {
	for _, node := range c.Workers {
		if node.Status.IP == ip {
			return node
		}
	}
	return nil
}

func (c *Cluster) GetNodeByName(name string) *Node {
	if c.Master != nil && c.Master.Name == name {
		return c.Master
	}
	for _, node := range c.Workers {
		if node != nil && node.Name == name {
			return node
		}
	}
	return nil
}

func (c *Cluster) RemoveNode(name string) {
	clusterLock.Lock()
	defer clusterLock.Unlock()
	if c.Master.Name == name {
		c.Master = nil
	}
	for i, node := range c.Workers {
		if node.Name == name {
			c.Workers = slices.Delete(c.Workers, i, i+1)
			break
		}
	}
}

func (c *Cluster) AddNode(node *Node) {
	clusterLock.Lock()
	defer clusterLock.Unlock()
	c.Workers = append(c.Workers, node)
}

func (c *Cluster) SetMaster(node *Node) {
	c.Master = node
}

func (c *Cluster) GenMasterName() string {
	var nodeName string
	for {
		nodeName = fmt.Sprintf("%s-master-%s", c.Name, GetRandomString(6))
		if c.GetNodeByName(nodeName) == nil {
			break
		}
	}
	return nodeName
}

func (c *Cluster) Prefix() string {
	return fmt.Sprintf("%s-", c.Name)
}

func (c *Cluster) GenWorkerName() string {
	var nodeName string
	for {
		nodeName = fmt.Sprintf("%s-worker-%s", c.Name, GetRandomString(6))
		if c.GetNodeByName(nodeName) == nil {
			break
		}
	}
	return nodeName
}

// Save saves cluster information to a file
func (c *Cluster) Save() error {
	// Create the .ohmykube directory
	clusterDir, err := ClusterDir(c.Name)
	if err != nil {
		return fmt.Errorf("failed to get cluster directory: %w", err)
	}
	if err := os.MkdirAll(clusterDir, 0755); err != nil {
		return fmt.Errorf("failed to create .ohmykube directory: %w", err)
	}

	// Save cluster information to YAML file
	clusterYaml := filepath.Join(clusterDir, "cluster.yaml")
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to serialize cluster information: %w", err)
	}

	if err := os.WriteFile(clusterYaml, data, 0644); err != nil {
		return fmt.Errorf("failed to save cluster information to %s: %w", clusterYaml, err)
	}

	return nil
}

// Load loads cluster information from a file
func Load(name string) (*Cluster, error) {
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

func CheckExists(name string) bool {
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

func GetRandomString(length int) string {
	localRand.Shuffle(len(letters), func(i, j int) {
		letters[i], letters[j] = letters[j], letters[i]
	})
	return string(letters[:length])
}

var letters = []byte("abcdefghijklmnopqrstuvwxyzZ1234567890")
