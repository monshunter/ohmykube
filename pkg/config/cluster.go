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
	ConditionTypeAuthInitialized ConditionType = "AuthInitialized"
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

func (n *Node) SetStatus(status NodeStatus) {
	n.Status = status
}

func (n *Node) SetInternalStatus(status NodeInternalStatus) {
	n.Status.NodeInternalStatus = status
}

func (n *Node) SetConditions(conditions []Condition) {
	n.Status.Conditions = conditions
}

func (n *Node) Arch() string {
	return n.Status.Arch
}

func (n *Node) OS() string {
	return n.Status.OS
}

func (n *Node) Kernel() string {
	return n.Status.Kernel
}

func (n *Node) Release() string {
	return n.Status.Release
}

func (n *Node) IPs() []string {
	return n.Status.IPs
}

func (n *Node) IP() string {
	return n.Status.IP
}

func (n *Node) IPv6() string {
	return n.Status.IPv6
}

func (n *Node) Hostname() string {
	return n.Status.Hostname
}

func (n *Node) Phase() Phase {
	return n.Status.Phase
}

func (n *Node) IsRunning() bool {
	return n.Status.Phase == PhaseRunning
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
	for _, group := range c.Status.Nodes {
		for _, member := range group.Members {
			if !c.hasNodeCondition(member, conditionType, status) {
				return false
			}
		}
	}
	return true
}

// hasNodeCondition checks if a node member has a specific condition
func (c *Cluster) hasNodeCondition(member NodeGroupMember, conditionType ConditionType, status ConditionStatus) bool {
	for _, condition := range member.Conditions {
		if condition.Type == conditionType && condition.Status == status {
			return true
		}
	}
	return false
}

// GetNodeGroupByID returns a node group status by its group ID
func (c *Cluster) GetNodeGroupByID(groupID int) *NodeGroupStatus {
	for i := range c.Status.Nodes {
		if c.Status.Nodes[i].GroupID == groupID {
			return &c.Status.Nodes[i]
		}
	}
	return nil
}

// GetOrCreateNodeGroup returns an existing node group or creates a new one
func (c *Cluster) GetOrCreateNodeGroup(groupID int) *NodeGroupStatus {
	if group := c.GetNodeGroupByID(groupID); group != nil {
		return group
	}

	// Create new node group
	newGroup := NodeGroupStatus{
		GroupID: groupID,
		Desire:  0,
		Created: 0,
		Running: 0,
		Members: []NodeGroupMember{},
	}
	c.Status.Nodes = append(c.Status.Nodes, newGroup)
	return &c.Status.Nodes[len(c.Status.Nodes)-1]
}

// Note: AddNodeToGroup and updateNodeGroupMember methods have been removed
// as they were designed for the legacy Node structure.
// Use CreateNodeInGroup and direct member manipulation instead.

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

// NetworkingConfig defines networking configuration for the cluster
type NetworkingConfig struct {
	ProxyMode     string `yaml:"proxyMode,omitempty"`
	CNI           string `yaml:"cni,omitempty"`
	PodSubnet     string `yaml:"podSubnet,omitempty"`
	ServiceSubnet string `yaml:"serviceSubnet,omitempty"`
	LoadBalancer  string `yaml:"loadbalancer,omitempty"`
}

// StorageConfig defines storage configuration for the cluster
type StorageConfig struct {
	CSI string `yaml:"csi,omitempty"`
}

// ResourceRequests defines resource requirements for nodes
type ResourceRequests struct {
	CPU     string `yaml:"cpu,omitempty"`
	Memory  string `yaml:"memory,omitempty"`
	Storage string `yaml:"storage,omitempty"`
}

// NodeGroupSpec defines a group of nodes with the same configuration
type NodeGroupSpec struct {
	Replica   int              `yaml:"replica,omitempty"`
	GroupID   int              `yaml:"groupid,omitempty"`
	Template  string           `yaml:"template,omitempty"`
	Resources ResourceRequests `yaml:"resources,omitempty"`
}

// NodesConfig defines the node configuration for master and workers
type NodesConfig struct {
	Master  []NodeGroupSpec `yaml:"master,omitempty"`
	Workers []NodeGroupSpec `yaml:"workers,omitempty"`
}

type ClusterSpec struct {
	KubernetesVersion string           `yaml:"kubernetesVersion,omitempty"`
	Provider          string           `yaml:"provider,omitempty"`
	Networking        NetworkingConfig `yaml:"networking,omitempty"`
	Storage           StorageConfig    `yaml:"storage,omitempty"`
	Nodes             NodesConfig      `yaml:"nodes,omitempty"`
}

// NodeGroupMember represents a single node within a node group
type NodeGroupMember struct {
	Name       string      `yaml:"name,omitempty"`
	Phase      Phase       `yaml:"phase,omitempty"`
	Hostname   string      `yaml:"hostname,omitempty"`
	IP         string      `yaml:"ip,omitempty"`
	Release    string      `yaml:"release,omitempty"`
	Kernel     string      `yaml:"kernel,omitempty"`
	Arch       string      `yaml:"arch,omitempty"`
	OS         string      `yaml:"os,omitempty"`
	Conditions []Condition `yaml:"conditions,omitempty"`
}

// NodeGroupStatus represents the status of a node group
type NodeGroupStatus struct {
	GroupID int               `yaml:"groupid,omitempty"`
	Desire  int               `yaml:"desire,omitempty"`
	Created int               `yaml:"created,omitempty"`
	Running int               `yaml:"running,omitempty"`
	Members []NodeGroupMember `yaml:"members,omitempty"`
}

type ClusterStatus struct {
	Phase      ClusterPhase      `yaml:"phase,omitempty"`
	Auth       Auth              `yaml:"auth,omitempty"`
	Images     Images            `yaml:"images,omitempty"` // Cluster images (names correspond to global cache)
	Nodes      []NodeGroupStatus `yaml:"nodes,omitempty"`  // New node group status
	Conditions []Condition       `yaml:"conditions,omitempty"`
}

// Image represents a simple image reference
// The Name field corresponds to the name in the global image cache
type Image struct {
	Name string `yaml:"name,omitempty"` // Corresponds to global cache image name (e.g., "registry.k8s.io//kube-apiserver:v1.33.0_arm64")
}

type Images []Image

// getTemplateOrDefault returns the template or default if empty
func getTemplateOrDefault(template string) string {
	if template == "" {
		return "ubuntu-24.04" // Default template
	}
	return template
}

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
			KubernetesVersion: config.KubernetesVersion,
			Provider:          config.Provider,
			Networking: NetworkingConfig{
				ProxyMode:     config.ProxyMode,
				CNI:           config.CNI,
				PodSubnet:     "10.244.0.0/16", // Default pod subnet
				ServiceSubnet: "10.96.0.0/12",  // Default service subnet
				LoadBalancer:  config.LB,
			},
			Storage: StorageConfig{
				CSI: config.CSI,
			},
			Nodes: NodesConfig{
				Master: []NodeGroupSpec{
					{
						Replica:  1,
						GroupID:  1,
						Template: getTemplateOrDefault(config.Template),
						Resources: ResourceRequests{
							CPU:     fmt.Sprintf("%d", config.Master.CPU),
							Memory:  fmt.Sprintf("%dGi", config.Master.Memory),
							Storage: fmt.Sprintf("%dGi", config.Master.Disk),
						},
					},
				},
				Workers: []NodeGroupSpec{},
			},
		},
		Status: ClusterStatus{
			Phase: ClusterPhasePending,
			Nodes: []NodeGroupStatus{},
		},
	}

	// Create worker node group (combine all workers into one group)
	if len(config.Workers) > 0 {
		// Use the first worker's configuration as the template
		firstWorker := config.Workers[0]
		cls.Spec.Nodes.Workers = append(cls.Spec.Nodes.Workers, NodeGroupSpec{
			Replica:  len(config.Workers), // Total number of worker nodes
			GroupID:  2,                   // Worker group ID
			Template: getTemplateOrDefault(config.Template),
			Resources: ResourceRequests{
				CPU:     fmt.Sprintf("%d", firstWorker.CPU),
				Memory:  fmt.Sprintf("%dGi", firstWorker.Memory),
				Storage: fmt.Sprintf("%dGi", firstWorker.Disk),
			},
		})
	}

	return cls
}

// InitializeNodeGroupsFromSpec initializes node group status from spec
func (c *Cluster) InitializeNodeGroupsFromSpec() {
	// Clear existing node groups
	c.Status.Nodes = []NodeGroupStatus{}

	// Initialize master node groups
	for _, masterSpec := range c.Spec.Nodes.Master {
		group := NodeGroupStatus{
			GroupID: masterSpec.GroupID,
			Desire:  masterSpec.Replica,
			Created: 0,
			Running: 0,
			Members: []NodeGroupMember{},
		}
		c.Status.Nodes = append(c.Status.Nodes, group)
	}

	// Initialize worker node groups
	for _, workerSpec := range c.Spec.Nodes.Workers {
		group := NodeGroupStatus{
			GroupID: workerSpec.GroupID,
			Desire:  workerSpec.Replica,
			Created: 0,
			Running: 0,
			Members: []NodeGroupMember{},
		}
		c.Status.Nodes = append(c.Status.Nodes, group)
	}
}

// SetNodeCondition sets a condition for a specific node
func (c *Cluster) SetNodeCondition(nodeName string, conditionType ConditionType, status ConditionStatus, reason, message string) {
	for i := range c.Status.Nodes {
		group := &c.Status.Nodes[i]
		for j := range group.Members {
			if group.Members[j].Name == nodeName {
				c.setNodeMemberCondition(&group.Members[j], conditionType, status, reason, message)
				return
			}
		}
	}
}

// setNodeMemberCondition sets or updates a condition for a node member
func (c *Cluster) setNodeMemberCondition(member *NodeGroupMember, conditionType ConditionType, status ConditionStatus, reason, message string) {
	condition := NewCondition(conditionType, status, reason, message)

	// Find and update existing condition or append new one
	for i, c := range member.Conditions {
		if c.Type == conditionType {
			// Only update if status changed to avoid unnecessary updates
			if c.Status != status {
				condition.LastTransitionTime = time.Now()
				member.Conditions[i] = condition
			} else {
				// Just update reason and message if status didn't change
				member.Conditions[i].Reason = reason
				member.Conditions[i].Message = message
			}
			return
		}
	}

	// Condition not found, append new one
	member.Conditions = append(member.Conditions, condition)
}

// GetNodeCondition gets a condition by type from a node
func (c *Cluster) GetNodeCondition(nodeName string, conditionType ConditionType) (Condition, bool) {
	for _, group := range c.Status.Nodes {
		for _, member := range group.Members {
			if member.Name == nodeName {
				return FindCondition(member.Conditions, conditionType)
			}
		}
	}
	return Condition{}, false
}

// HasNodeCondition checks if a node has a condition with the specified type and status
func (c *Cluster) HasNodeCondition(nodeName string, conditionType ConditionType, status ConditionStatus) bool {
	if condition, found := c.GetNodeCondition(nodeName, conditionType); found {
		return condition.Status == status
	}
	return false
}

// SetNodeIP sets the IP address for a specific node
func (c *Cluster) SetNodeIP(nodeName string, ip string) {
	for i := range c.Status.Nodes {
		group := &c.Status.Nodes[i]
		for j := range group.Members {
			if group.Members[j].Name == nodeName {
				group.Members[j].IP = ip
				return
			}
		}
	}
}

// SetNodeHostname sets the hostname for a specific node
func (c *Cluster) SetNodeHostname(nodeName string, hostname string) {
	for i := range c.Status.Nodes {
		group := &c.Status.Nodes[i]
		for j := range group.Members {
			if group.Members[j].Name == nodeName {
				group.Members[j].Hostname = hostname
				return
			}
		}
	}
}

// SetNodeSystemInfo sets system information for a specific node
func (c *Cluster) SetNodeSystemInfo(nodeName string, release, kernel, arch, os string) {
	for i := range c.Status.Nodes {
		group := &c.Status.Nodes[i]
		for j := range group.Members {
			if group.Members[j].Name == nodeName {
				group.Members[j].Release = release
				group.Members[j].Kernel = kernel
				group.Members[j].Arch = arch
				group.Members[j].OS = os
				return
			}
		}
	}
}

// CreateNodeInGroup creates a new node in the specified group
func (c *Cluster) CreateNodeInGroup(groupID int, nodeName string) *NodeGroupMember {
	group := c.GetOrCreateNodeGroup(groupID)

	// Check if node already exists
	for i := range group.Members {
		if group.Members[i].Name == nodeName {
			return &group.Members[i]
		}
	}

	// Create new member
	member := NodeGroupMember{
		Name:       nodeName,
		Phase:      PhasePending,
		Conditions: []Condition{},
	}

	group.Members = append(group.Members, member)
	group.Created++

	return &group.Members[len(group.Members)-1]
}

// GetKubernetesVersion returns the Kubernetes version
func (c *Cluster) GetKubernetesVersion() string {
	return c.Spec.KubernetesVersion
}

// GetProxyMode returns the proxy mode from networking config
func (c *Cluster) GetProxyMode() string {
	return c.Spec.Networking.ProxyMode
}

// GetCNI returns the CNI from networking config
func (c *Cluster) GetCNI() string {
	return c.Spec.Networking.CNI
}

// GetCSI returns the CSI from storage config
func (c *Cluster) GetCSI() string {
	return c.Spec.Storage.CSI
}

// GetLoadBalancer returns the load balancer from networking config
func (c *Cluster) GetLoadBalancer() string {
	return c.Spec.Networking.LoadBalancer
}

// GetPodSubnet returns the pod subnet from networking config
func (c *Cluster) GetPodSubnet() string {
	if c.Spec.Networking.PodSubnet != "" {
		return c.Spec.Networking.PodSubnet
	}
	return "10.244.0.0/16" // Default pod subnet
}

// GetServiceSubnet returns the service subnet from networking config
func (c *Cluster) GetServiceSubnet() string {
	if c.Spec.Networking.ServiceSubnet != "" {
		return c.Spec.Networking.ServiceSubnet
	}
	return "10.96.0.0/12" // Default service subnet
}

// GetNodeGroupSpecs returns all node group specifications
func (c *Cluster) GetNodeGroupSpecs() []NodeGroupSpec {
	var specs []NodeGroupSpec
	specs = append(specs, c.Spec.Nodes.Master...)
	specs = append(specs, c.Spec.Nodes.Workers...)
	return specs
}

// GetMasterNodeGroupSpecs returns master node group specifications
func (c *Cluster) GetMasterNodeGroupSpecs() []NodeGroupSpec {
	return c.Spec.Nodes.Master
}

// GetWorkerNodeGroupSpecs returns worker node group specifications
func (c *Cluster) GetWorkerNodeGroupSpecs() []NodeGroupSpec {
	return c.Spec.Nodes.Workers
}

// GetTotalDesiredNodes returns the total number of desired nodes across all groups
func (c *Cluster) GetTotalDesiredNodes() int {
	total := 0
	for _, spec := range c.GetNodeGroupSpecs() {
		total += spec.Replica
	}
	return total
}

// GetTotalRunningNodes returns the total number of running nodes across all groups
func (c *Cluster) GetTotalRunningNodes() int {
	total := 0
	for _, group := range c.Status.Nodes {
		total += group.Running
	}
	return total
}

// IsClusterFullyRunning checks if all desired nodes are running
func (c *Cluster) IsClusterFullyRunning() bool {
	for _, group := range c.Status.Nodes {
		if group.Running < group.Desire {
			return false
		}
	}
	return true
}

// FindMatchingWorkerGroup finds a worker node group with matching template and resources
func (c *Cluster) FindMatchingWorkerGroup(template string, resources ResourceRequests) *NodeGroupSpec {
	for i := range c.Spec.Nodes.Workers {
		worker := &c.Spec.Nodes.Workers[i]
		if worker.Template == template &&
			worker.Resources.CPU == resources.CPU &&
			worker.Resources.Memory == resources.Memory &&
			worker.Resources.Storage == resources.Storage {
			return worker
		}
	}
	return nil
}

// AddNodeToWorkerGroup adds a node to an existing worker group or creates a new one
func (c *Cluster) AddNodeToWorkerGroup(template string, resources ResourceRequests) int {
	// Try to find matching existing group
	if matchingGroup := c.FindMatchingWorkerGroup(template, resources); matchingGroup != nil {
		// Increase replica count in existing group
		matchingGroup.Replica++

		// Update corresponding status group desire count
		for i := range c.Status.Nodes {
			if c.Status.Nodes[i].GroupID == matchingGroup.GroupID {
				c.Status.Nodes[i].Desire++
				break
			}
		}

		return matchingGroup.GroupID
	}

	// No matching group found, create new one
	newGroupID := c.getNextAvailableGroupID()
	newGroup := NodeGroupSpec{
		Replica:   1,
		GroupID:   newGroupID,
		Template:  template,
		Resources: resources,
	}

	// Add to spec
	c.Spec.Nodes.Workers = append(c.Spec.Nodes.Workers, newGroup)

	// Add corresponding status group
	statusGroup := NodeGroupStatus{
		GroupID: newGroupID,
		Desire:  1,
		Created: 0,
		Running: 0,
		Members: []NodeGroupMember{},
	}
	c.Status.Nodes = append(c.Status.Nodes, statusGroup)

	return newGroupID
}

// getNextAvailableGroupID returns the next available group ID
func (c *Cluster) getNextAvailableGroupID() int {
	maxGroupID := 1 // Start from 2 since 1 is reserved for master

	// Check spec groups
	for _, masterSpec := range c.Spec.Nodes.Master {
		if masterSpec.GroupID > maxGroupID {
			maxGroupID = masterSpec.GroupID
		}
	}
	for _, workerSpec := range c.Spec.Nodes.Workers {
		if workerSpec.GroupID > maxGroupID {
			maxGroupID = workerSpec.GroupID
		}
	}

	return maxGroupID + 1
}

func (c *Cluster) GetMasterIP() string {
	// Find master node in group 1
	for _, group := range c.Status.Nodes {
		if group.GroupID == 1 && len(group.Members) > 0 {
			return group.Members[0].IP
		}
	}
	return ""
}

func (c *Cluster) GetMasterName() string {
	// Find master node in group 1
	for _, group := range c.Status.Nodes {
		if group.GroupID == 1 && len(group.Members) > 0 {
			return group.Members[0].Name
		}
	}
	return ""
}

func (c *Cluster) GetWorkerNames() []string {
	var names []string
	// Find worker nodes in groups other than 1
	for _, group := range c.Status.Nodes {
		if group.GroupID != 1 {
			for _, member := range group.Members {
				names = append(names, member.Name)
			}
		}
	}
	return names
}

func (c *Cluster) Nodes2IPsMap() map[string]string {
	ips := make(map[string]string)
	for _, group := range c.Status.Nodes {
		for _, member := range group.Members {
			ips[member.Name] = member.IP
		}
	}
	return ips
}

func (c *Cluster) GetNodeByIP(ip string) *NodeGroupMember {
	for _, group := range c.Status.Nodes {
		for i := range group.Members {
			if group.Members[i].IP == ip {
				return &group.Members[i]
			}
		}
	}
	return nil
}

func (c *Cluster) GetNodeByName(name string) *NodeGroupMember {
	for _, group := range c.Status.Nodes {
		for i := range group.Members {
			if group.Members[i].Name == name {
				return &group.Members[i]
			}
		}
	}
	return nil
}

func (c *Cluster) GetNodeOrNew(name string, role string, cpu int, memory int, disk int) *NodeGroupMember {
	node := c.GetNodeByName(name)
	if node == nil {
		if name == "" {
			name = c.GenNodeName(role)
		}
		// Create new node member
		member := &NodeGroupMember{
			Name:  name,
			Phase: PhasePending,
		}

		// Determine group ID based on role
		groupID := 1 // Master
		if role == RoleWorker {
			groupID = c.getNextWorkerGroupID()
		}

		// Add to appropriate group
		group := c.GetOrCreateNodeGroup(groupID)
		group.Members = append(group.Members, *member)
		group.Created++

		return &group.Members[len(group.Members)-1]
	}
	return node
}

func (c *Cluster) RemoveNode(name string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for i := range c.Status.Nodes {
		group := &c.Status.Nodes[i]
		for j, member := range group.Members {
			if member.Name == name {
				// Remove member from group
				group.Members = slices.Delete(group.Members, j, j+1)
				group.Created--
				if member.Phase == PhaseRunning {
					group.Running--
				}

				// Update corresponding spec replica count
				group.Desire--
				c.updateSpecReplicaForGroup(group.GroupID, group.Desire)

				return
			}
		}
	}
}

// updateSpecReplicaForGroup updates the replica count in spec for a given group
func (c *Cluster) updateSpecReplicaForGroup(groupID int, newReplica int) {
	// Update master spec
	for i := range c.Spec.Nodes.Master {
		if c.Spec.Nodes.Master[i].GroupID == groupID {
			c.Spec.Nodes.Master[i].Replica = newReplica
			return
		}
	}

	// Update worker spec
	for i := range c.Spec.Nodes.Workers {
		if c.Spec.Nodes.Workers[i].GroupID == groupID {
			c.Spec.Nodes.Workers[i].Replica = newReplica
			return
		}
	}
}

// getNextWorkerGroupID returns the next available group ID for worker nodes
func (c *Cluster) getNextWorkerGroupID() int {
	maxGroupID := 1 // Start from 2 since 1 is reserved for master
	for _, group := range c.Status.Nodes {
		if group.GroupID > maxGroupID {
			maxGroupID = group.GroupID
		}
	}
	return maxGroupID + 1
}

func (c *Cluster) SetPhase(phase ClusterPhase) {
	c.Status.Phase = phase
}

func (c *Cluster) SetPhaseForNode(nodeName string, phase Phase) {
	for i := range c.Status.Nodes {
		group := &c.Status.Nodes[i]
		for j := range group.Members {
			if group.Members[j].Name == nodeName {
				oldPhase := group.Members[j].Phase
				group.Members[j].Phase = phase

				// Update group counters
				if oldPhase == PhaseRunning && phase != PhaseRunning {
					group.Running--
				} else if oldPhase != PhaseRunning && phase == PhaseRunning {
					group.Running++
				}
				return
			}
		}
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
