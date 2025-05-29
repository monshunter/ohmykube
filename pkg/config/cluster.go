package config

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

var localRand = rand.New(rand.NewSource(time.Now().UnixNano()))

type Phase string

const (
	PhasePending Phase = "Pending"
	PhaseRunning Phase = "Running"
	PhaseStopped Phase = "Stopped"
	PhaseFailed  Phase = "Failed"
	PhaseUnknown Phase = "Unknown"
)

type ClusterPhase string

const (
	ClusterPhasePending ClusterPhase = "Pending"
	ClusterPhaseRunning ClusterPhase = "Running"
	ClusterPhaseFailed  ClusterPhase = "Failed"
	ClusterPhaseUnknown ClusterPhase = "Unknown"
)

const (
	KindCluster = "Cluster"
	ApiVersion  = "ohmykube.dev/v1alpha1"
)

type Metadata struct {
	Name        string            `yaml:"name,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
	Taints      []Taint           `yaml:"taints,omitempty"`
	Deleted     bool              `yaml:"deleted,omitempty"`
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
	NodeInternalStatus `yaml:",inline"`
	Conditions         []Condition `yaml:"conditions,omitempty"`
}

type NodeInternalStatus struct {
	// Internal status fields
	Phase    Phase  `yaml:"phase,omitempty"`
	Hostname string `yaml:"hostname,omitempty"`
	// IP ipv4 address
	IP      string   `yaml:"ip,omitempty"`
	IPv6    string   `yaml:"ipv6,omitempty"`
	IPs     []string `yaml:"ips,omitempty"`
	Release string   `yaml:"release,omitempty"`
	Kernel  string   `yaml:"kernel,omitempty"`
	Arch    string   `yaml:"arch,omitempty"`
	OS      string   `yaml:"os,omitempty"`
}

type ConditionType string

const (

	// Workflow stage conditions for nodes
	ConditionTypeNodeReady       ConditionType = "NodeReady"
	ConditionTypeVMCreated       ConditionType = "VMCreated"
	ConditionTypeEnvironmentInit ConditionType = "EnvironmentInitialized"
	ConditionTypeKubeInitialized ConditionType = "KubernetesInitialized"
	ConditionTypeJoinedCluster   ConditionType = "JoinedCluster"

	// Cluster-level workflow conditions
	ConditionTypeMasterInitialized ConditionType = "MasterInitialized"
	ConditionTypeWorkersJoined     ConditionType = "WorkersJoined"
	ConditionTypeCNIInstalled      ConditionType = "CNIInstalled"
	ConditionTypeCSIInstalled      ConditionType = "CSIInstalled"
	ConditionTypeLBInstalled       ConditionType = "LoadBalancerInstalled"
	ConditionTypeClusterReady      ConditionType = "ClusterReady"
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
		Type:               t,
		Status:             s,
		Reason:             r,
		Message:            m,
		LastTransitionTime: time.Now(),
	}
}

// FindCondition finds a condition by type in a slice of conditions
func FindCondition(conditions []Condition, conditionType ConditionType) (Condition, bool) {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition, true
		}
	}
	return Condition{}, false
}

// SetNodeCondition sets or updates a condition in a node's status
func (n *Node) SetCondition(conditionType ConditionType, status ConditionStatus, reason, message string) {
	condition := NewCondition(conditionType, status, reason, message)

	// Find and update existing condition or append new one
	for i, c := range n.Status.Conditions {
		if c.Type == conditionType {
			// Only update if status changed to avoid unnecessary updates
			if c.Status != status {
				condition.LastTransitionTime = time.Now()
				n.Status.Conditions[i] = condition
			} else {
				// Just update reason and message if status didn't change
				n.Status.Conditions[i].Reason = reason
				n.Status.Conditions[i].Message = message
			}
			return
		}
	}

	// Condition not found, append new one
	n.Status.Conditions = append(n.Status.Conditions, condition)
}

// GetCondition gets a condition by type from a node
func (n *Node) GetCondition(conditionType ConditionType) (Condition, bool) {
	return FindCondition(n.Status.Conditions, conditionType)
}

// HasCondition checks if a node has a condition with the specified type and status
func (n *Node) HasCondition(conditionType ConditionType, status ConditionStatus) bool {
	if condition, found := n.GetCondition(conditionType); found {
		return condition.Status == status
	}
	return false
}

func (n *Node) SetPhase(phase Phase) {
	n.Status.Phase = phase
}

func (n *Node) SetIP(ip string) {
	n.Status.IP = ip
}

func (n *Node) SetIPv6(ipv6 string) {
	n.Status.IPv6 = ipv6
}

func (n *Node) SetIPs(ips []string) {
	n.Status.IPs = ips
}

func (n *Node) SetHostname(hostname string) {
	n.Status.Hostname = hostname
}

func (n *Node) SetRelease(release string) {
	n.Status.Release = release
}

func (n *Node) SetKernel(kernel string) {
	n.Status.Kernel = kernel
}

func (n *Node) SetArch(arch string) {
	n.Status.Arch = arch
}

func (n *Node) SetOS(os string) {
	n.Status.OS = os
}

// SetClusterCondition sets or updates a condition in a cluster's status
func (c *Cluster) SetCondition(conditionType ConditionType, status ConditionStatus, reason, message string) {
	condition := NewCondition(conditionType, status, reason, message)

	// Find and update existing condition or append new one
	for i, cond := range c.Status.Conditions {
		if cond.Type == conditionType {
			// Only update if status changed to avoid unnecessary updates
			if cond.Status != status {
				condition.LastTransitionTime = time.Now()
				c.Status.Conditions[i] = condition
			} else {
				// Just update reason and message if status didn't change
				c.Status.Conditions[i].Reason = reason
				c.Status.Conditions[i].Message = message
			}
			return
		}
	}

	// Condition not found, append new one
	c.Status.Conditions = append(c.Status.Conditions, condition)
}

// GetCondition gets a condition by type from a cluster
func (c *Cluster) GetCondition(conditionType ConditionType) (Condition, bool) {
	return FindCondition(c.Status.Conditions, conditionType)
}

// HasCondition checks if a cluster has a condition with the specified type and status
func (c *Cluster) HasCondition(conditionType ConditionType, status ConditionStatus) bool {
	if condition, found := c.GetCondition(conditionType); found {
		return condition.Status == status
	}
	return false
}

func (c *Cluster) HasAllNodeCondition(conditionType ConditionType, status ConditionStatus) bool {
	if c.Spec.Master != nil && !c.Spec.Master.HasCondition(conditionType, status) {
		return false
	}
	for _, node := range c.Spec.Workers {
		if node != nil && !node.HasCondition(conditionType, status) {
			return false
		}
	}
	return true
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

func (n *Node) SetStatus(status NodeStatus) {
	n.Status = status
}

func (n *Node) SetInternalStatus(status NodeInternalStatus) {
	n.Status.NodeInternalStatus = status
}

type Auth struct {
	User    string `yaml:"user"`
	Port    string `yaml:"port"`
	KeyFile string `yaml:"keyFile"`
}

// Cluster stores cluster information
type Cluster struct {
	ApiVersion string `yaml:"apiVersion,omitempty"`
	Kind       string `yaml:"kind,omitempty"`
	Metadata   `yaml:"metadata,omitempty"`
	Spec       ClusterSpec   `yaml:"spec,omitempty"`
	Status     ClusterStatus `yaml:"status,omitempty"`
	lock       sync.RWMutex
}

type ClusterSpec struct {
	K8sVersion string  `yaml:"k8sVersion,omitempty"`
	Launcher   string  `yaml:"launcher,omitempty"`
	ProxyMode  string  `yaml:"proxyMode,omitempty"`
	Master     *Node   `yaml:"master,omitempty"`
	Workers    []*Node `yaml:"workers,omitempty"`
	CNI        string  `yaml:"cni,omitempty"`
	CSI        string  `yaml:"csi,omitempty"`
	LB         string  `yaml:"lb,omitempty"`
}

type ClusterStatus struct {
	Phase      ClusterPhase `yaml:"phase,omitempty"`
	Auth       Auth         `yaml:"auth,omitempty"`
	Images     Images       `yaml:"images,omitempty"` // Cluster images (names correspond to global cache)
	Conditions []Condition  `yaml:"conditions,omitempty"`
}

// Image represents a simple image reference
// The Name field corresponds to the name in the global image cache
type Image struct {
	Name string `yaml:"name,omitempty"` // Corresponds to global cache image name (e.g., "registry.k8s.io//kube-apiserver:v1.33.0_arm64")
}

type Images []Image

func NewCluster(config *Config) *Cluster {
	cls := &Cluster{
		ApiVersion: ApiVersion,
		Kind:       KindCluster,
		Metadata: Metadata{
			Name: config.Name,
			Annotations: map[string]string{
				"ohmykube.dev/created-at": time.Now().Format(time.RFC3339),
			},

			Labels: map[string]string{
				"ohmykube.dev/cluster": config.Name,
			},
		},
		Spec: ClusterSpec{
			K8sVersion: config.KubernetesVersion,
			Launcher:   config.LauncherType,
			ProxyMode:  config.ProxyMode,
			Workers:    make([]*Node, 0),
			CNI:        config.CNI,
			CSI:        config.CSI,
			LB:         config.LB,
		},
		Status: ClusterStatus{
			Phase: ClusterPhasePending,
		},
	}
	cls.Spec.Master = NewNode(cls.GenNodeName(RoleMaster), RoleMaster,
		config.Master.CPU, config.Master.Memory, config.Master.Disk)
	for _, worker := range config.Workers {
		cls.Spec.Workers = append(cls.Spec.Workers, NewNode(cls.GenNodeName(RoleWorker),
			RoleWorker, worker.CPU, worker.Memory, worker.Disk))
	}
	return cls
}

func (c *Cluster) GetMasterIP() string {
	if c.Spec.Master == nil {
		return ""
	}
	return c.Spec.Master.Status.IP
}

func (c *Cluster) GetMasterName() string {
	if c.Spec.Master == nil {
		return ""
	}
	return c.Spec.Master.Name
}

func (c *Cluster) GetWorkerNames() []string {
	names := make([]string, len(c.Spec.Workers))
	for i, node := range c.Spec.Workers {
		names[i] = node.Name
	}
	return names
}

func (c *Cluster) Nodes2IPsMap() map[string]string {
	ips := make(map[string]string)
	ips[c.Spec.Master.Name] = c.Spec.Master.Status.IP
	for _, node := range c.Spec.Workers {
		ips[node.Name] = node.Status.IP
	}
	return ips
}

func (c *Cluster) GetNodeByIP(ip string) *Node {
	for _, node := range c.Spec.Workers {
		if node.Status.IP == ip {
			return node
		}
	}
	return nil
}

func (c *Cluster) GetNodeByName(name string) *Node {
	if c.Spec.Master != nil && c.Spec.Master.Name == name {
		return c.Spec.Master
	}
	for _, node := range c.Spec.Workers {
		if node != nil && node.Name == name {
			return node
		}
	}
	return nil
}

func (c *Cluster) GetNodeOrNew(name string, role string, cpu int, memory int, disk int) *Node {
	node := c.GetNodeByName(name)
	if node == nil {
		if name == "" {
			name = c.GenNodeName(role)
		}
		node = NewNode(name, role, cpu, memory, disk)
		if role == RoleMaster {
			c.SetMaster(node)
		} else {
			c.AddNode(node)
		}
	}
	return node
}

func (c *Cluster) RemoveNode(name string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.Spec.Master.Name == name {
		c.Spec.Master = nil
	}
	for i, node := range c.Spec.Workers {
		if node.Name == name {
			c.Spec.Workers = slices.Delete(c.Spec.Workers, i, i+1)
			break
		}
	}
}

func (c *Cluster) AddNode(node *Node) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.Spec.Workers = append(c.Spec.Workers, node)
}

func (c *Cluster) SetMaster(node *Node) {
	c.Spec.Master = node
}

func (c *Cluster) SetPhase(phase ClusterPhase) {
	c.Status.Phase = phase
}

func (c *Cluster) SetPhaseForNode(nodeName string, phase Phase) {
	node := c.GetNodeByName(nodeName)
	if node != nil {
		node.SetPhase(phase)
	}
}

func (c *Cluster) Prefix() string {
	return fmt.Sprintf("%s-", c.Name)
}

func (c *Cluster) GenNodeName(role string) string {
	var nodeName string
	for {
		nodeName = fmt.Sprintf("%s-%s-%s", c.Name, role, GetRandomString(6))
		if c.GetNodeByName(nodeName) == nil {
			break
		}
	}
	return nodeName
}

// RecordClusterImage records an image when it's being cached
// Implements interfaces.ClusterImageTracker
func (c *Cluster) RecordImage(imageName string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	// Check if image already exists
	for _, img := range c.Status.Images {
		if img.Name == imageName {
			return // Image already tracked
		}
	}

	// Add new image (we don't need arch since it's embedded in the image name)
	c.Status.Images = append(c.Status.Images, Image{
		Name: imageName,
	})
}

// GetClusterImageNames returns all recorded cluster image names
// Implements interfaces.ClusterImageTracker
func (c *Cluster) GetImageNames() []string {
	c.lock.RLock()
	defer c.lock.RUnlock()
	names := make([]string, len(c.Status.Images))
	for i, img := range c.Status.Images {
		names[i] = img.Name
	}
	return names
}

// Save saves cluster information to a file
func (c *Cluster) Save() error {
	if c.Deleted {
		return nil
	}
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

// Clear clears the cluster data but keeps the name
// Note: This doesn't set the pointer to nil, as that wouldn't affect the caller's pointer
func (c *Cluster) MarkDeleted() {
	if c == nil {
		return
	}
	c.Metadata.Deleted = true
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

var letters = []byte("abcdefghijklmnopqrstuvwxyz1234567890")
