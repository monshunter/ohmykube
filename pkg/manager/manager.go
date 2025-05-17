package manager

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/cni"
	"github.com/monshunter/ohmykube/pkg/csi"
	"github.com/monshunter/ohmykube/pkg/environment"
	"github.com/monshunter/ohmykube/pkg/kubeadm"
	"github.com/monshunter/ohmykube/pkg/kubeconfig"
	"github.com/monshunter/ohmykube/pkg/lb"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/multipass"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// Manager cluster manager
type Manager struct {
	Config             *cluster.Config
	MultipassClient    *multipass.Client
	KubeadmConfig      *kubeadm.KubeadmConfig
	SSHManager         *ssh.SSHManager
	Cluster            *cluster.Cluster
	InitOptions        environment.InitOptions
	CNIType            string // CNI type to use, default is flannel
	CSIType            string // CSI type to use, default is local-path-provisioner
	DownloadKubeconfig bool   // Whether to download kubeconfig to local
}

// NewManager creates a new cluster manager
func NewManager(config *cluster.Config, sshConfig *ssh.SSHConfig, cluster *cluster.Cluster) (*Manager, error) {
	mpClient, err := multipass.NewClient(config.Image, sshConfig.Password, sshConfig.GetSSHKey(), sshConfig.GetSSHPubKey())
	if err != nil {
		return nil, fmt.Errorf("failed to create Multipass client: %w", err)
	}

	// Create cluster information object
	if cluster == nil {
		cluster = clusterFromConfig(config)
	}

	manager := &Manager{
		Config:             config,
		MultipassClient:    mpClient,
		SSHManager:         ssh.NewSSHManager(cluster, sshConfig),
		Cluster:            cluster,
		InitOptions:        environment.DefaultInitOptions(),
		CNIType:            "flannel",                // Default to flannel
		CSIType:            "local-path-provisioner", // Default to local-path-provisioner
		DownloadKubeconfig: true,                     // Default to download kubeconfig
	}

	// SSH clients and KubeadmConfig will be created later when needed
	// KubeadmConfig will be set when initializing the Master node

	return manager, nil
}

func clusterFromConfig(config *cluster.Config) *cluster.Cluster {

	var master cluster.NodeInfo
	var workers []cluster.NodeInfo
	master = cluster.NodeInfo{
		Name:    config.Master.Name,
		Role:    cluster.RoleMaster,
		CPU:     config.Master.CPU,
		Memory:  config.Master.Memory,
		Disk:    config.Master.Disk,
		SSHUser: "root",
		SSHPort: "22",
	}

	for _, worker := range config.Workers {
		workers = append(workers, cluster.NodeInfo{
			Name:    worker.Name,
			Role:    cluster.RoleWorker,
			CPU:     worker.CPU,
			Memory:  worker.Memory,
			Disk:    worker.Disk,
			SSHUser: "root",
			SSHPort: "22",
		})
	}

	return cluster.NewCluster(config.Name, config.K8sVersion, master, workers)
}

// SetInitOptions sets environment initialization options
func (m *Manager) SetInitOptions(options environment.InitOptions) {
	m.InitOptions = options
}

// GetNodeIP gets the node IP address
func (m *Manager) GetNodeIP(nodeName string) (string, error) {
	// Retry a few times, as the node may need some time to fully start
	var err error
	maxRetries := 5
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		// Use multipass info command to get node information
		cmd := exec.Command("multipass", "info", nodeName, "--format", "json")
		output, err := cmd.Output()
		if err == nil {
			// Parse JSON output
			var info struct {
				Info map[string]struct {
					Ipv4 []string `json:"ipv4"`
				} `json:"info"`
			}

			if err := json.Unmarshal(output, &info); err != nil {
				return "", fmt.Errorf("failed to parse node information: %w", err)
			}

			nodeInfo, ok := info.Info[nodeName]
			if !ok || len(nodeInfo.Ipv4) == 0 {
				// If node info not found in this attempt, continue retrying
				time.Sleep(retryDelay)
				continue
			}

			return nodeInfo.Ipv4[0], nil
		}

		// If failed, wait a while before retrying
		log.Infof("Failed to get IP address for node %s, retrying (%d/%d)...", nodeName, i+1, maxRetries)
		time.Sleep(retryDelay)
	}

	return "", fmt.Errorf("failed to get IP address for node %s after %d retries: %w", nodeName, maxRetries, err)
}

// WaitForSSHReady waits for the node's SSH service to be ready
func (m *Manager) WaitForSSHReady(ip string, port string, maxRetries int) error {
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, port), 5*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}

		log.Infof("Waiting for SSH service to be ready (%s:%s), retrying (%d/%d)...", ip, port, i+1, maxRetries)
		time.Sleep(retryDelay)
	}

	return fmt.Errorf("timeout waiting for SSH service to be ready (%s:%s)", ip, port)
}

func (m *Manager) CloseSSHClient() {
	m.SSHManager.CloseAllClients()
}

func (m *Manager) AddNodeInfo(nodeName string, cpu int, memory int, disk int) error {
	extraInfo, err := m.GetExtraInfoFromRemote(nodeName)
	if err != nil {
		return fmt.Errorf("failed to get information for node %s: %w", nodeName, err)
	}
	nodeInfo := cluster.NewNodeInfo(nodeName, cluster.RoleWorker, cpu, memory, disk)
	nodeInfo.ExtraInfo = extraInfo
	m.Cluster.AddNode(nodeInfo)
	return nil
}

// UpdateClusterInfo updates cluster information
func (m *Manager) UpdateClusterInfo() error {
	extraInfomations := make([]cluster.NodeExtraInfo, 0, 1+len(m.Cluster.Workers))
	masterInfo, err := m.GetExtraInfoFromRemote(m.Cluster.Master.Name)
	if err != nil {
		return fmt.Errorf("failed to get Master node information: %w", err)
	}
	extraInfomations = append(extraInfomations, masterInfo)
	for i := range m.Cluster.Workers {
		workerInfo, err := m.GetExtraInfoFromRemote(m.Cluster.Workers[i].Name)
		if err != nil {
			return fmt.Errorf("failed to get Worker node information: %w", err)
		}
		extraInfomations = append(extraInfomations, workerInfo)
	}
	m.Cluster.UpdateWithExtraInfo(extraInfomations)
	// Save cluster information to local file
	return cluster.SaveClusterInfomation(m.Cluster)
}

func (m *Manager) GetExtraInfoFromRemote(nodeName string) (info cluster.NodeExtraInfo, err error) {
	// Get IP information
	ip, err := m.GetNodeIP(nodeName)
	if err != nil {
		return info, fmt.Errorf("failed to get IP for node %s: %w", nodeName, err)
	}

	// Fill in basic information
	info.Name = nodeName
	info.IP = ip
	info.Status = cluster.NodeStatusRunning
	sshClient, err := m.SSHManager.CreateClient(nodeName, ip)
	if err != nil {
		return info, fmt.Errorf("failed to create SSH client: %w", err)
	}
	// Get hostname
	hostnameCmd := "hostname"
	hostnameOutput, err := sshClient.RunCommand(hostnameCmd)
	if err == nil {
		info.Hostname = strings.TrimSpace(hostnameOutput)
	} else {
		log.Infof("Warning: failed to get hostname for node %s: %v", nodeName, err)
		return info, fmt.Errorf("failed to get hostname for node %s: %w", nodeName, err)
	}

	// Get OS distribution information
	releaseCmd := `cat /etc/os-release | grep PRETTY_NAME | cut -d'"' -f2`
	releaseOutput, err := sshClient.RunCommand(releaseCmd)
	if err == nil {
		info.Release = strings.TrimSpace(releaseOutput)
	} else {
		log.Infof("Warning: failed to get distribution information for node %s: %v", nodeName, err)
	}

	// Get kernel version
	kernelCmd := "uname -r"
	kernelOutput, err := sshClient.RunCommand(kernelCmd)
	if err == nil {
		info.Kernel = strings.TrimSpace(kernelOutput)
	} else {
		log.Infof("Warning: failed to get kernel version for node %s: %v", nodeName, err)
	}

	// Get system architecture
	archCmd := "uname -m"
	archOutput, err := sshClient.RunCommand(archCmd)
	if err == nil {
		info.Arch = strings.TrimSpace(archOutput)
	} else {
		log.Infof("Warning: failed to get system architecture for node %s: %v", nodeName, err)
	}

	// Get OS type
	osCmd := `uname -o`
	osOutput, err := sshClient.RunCommand(osCmd)
	if err == nil {
		info.OS = strings.TrimSpace(osOutput)
	} else {
		log.Infof("Warning: failed to get OS type for node %s: %v", nodeName, err)
	}

	return info, nil
}

// RunSSHCommand executes a command on a node via SSH
func (m *Manager) RunSSHCommand(nodeName, command string) (string, error) {
	return m.SSHManager.RunCommand(nodeName, command)
}

// SetCNIType sets the CNI type
func (m *Manager) SetCNIType(cniType string) {
	m.CNIType = cniType
}

// SetCSIType sets the CSI type
func (m *Manager) SetCSIType(csiType string) {
	m.CSIType = csiType
}

// SetDownloadKubeconfig sets whether to download kubeconfig to local
func (m *Manager) SetDownloadKubeconfig(download bool) {
	m.DownloadKubeconfig = download
}

// SetupKubeconfig configures local kubeconfig
func (m *Manager) SetupKubeconfig() (string, error) {
	// Get SSH client for the Master node
	sshClient, exists := m.SSHManager.GetClient(m.Config.Master.Name)
	if !exists {
		return "", fmt.Errorf("failed to get SSH client for Master node")
	}

	// Use unified kubeconfig download logic
	return kubeconfig.DownloadToLocal(sshClient, m.Config.Name, "")
}

// CreateCluster creates a new cluster
func (m *Manager) CreateCluster() error {
	log.Info("Starting to create Kubernetes cluster...")

	// 1. Create all nodes (in parallel)
	log.Info("Creating cluster nodes...")
	err := m.CreateClusterNodes()
	if err != nil {
		return fmt.Errorf("failed to create cluster nodes: %w", err)
	}

	// 2. Update cluster information
	log.Info("Getting cluster node information...")
	err = m.UpdateClusterInfo()
	if err != nil {
		return fmt.Errorf("failed to update cluster information: %w", err)
	}

	log.Info("Connecting to nodes via SSH as root user...")

	// 3. Initialize environment
	err = m.InitializeEnvironment()
	if err != nil {
		return fmt.Errorf("failed to initialize environment: %w", err)
	}
	// 4. Configure master node
	log.Info("Configuring Kubernetes Master node...")
	joinCommand, err := m.InitializeMaster()
	if err != nil {
		return fmt.Errorf("failed to initialize Master node: %w", err)
	}

	// 5. Configure worker nodes
	log.Info("Joining Worker nodes to the cluster...")
	err = m.JoinWorkerNodes(joinCommand)
	if err != nil {
		return fmt.Errorf("failed to join Worker nodes to the cluster: %w", err)
	}

	// 6. Install CNI
	if m.CNIType != "none" {
		log.Infof("Installing %s CNI...", m.CNIType)
		err = m.InstallCNI()
		if err != nil {
			return fmt.Errorf("failed to install CNI: %w", err)
		}
	} else {
		log.Info("Skipping CNI installation...")
	}

	// 7. Install CSI
	if m.CSIType != "none" {
		log.Infof("Installing %s CSI...", m.CSIType)
		err = m.InstallCSI()
		if err != nil {
			return fmt.Errorf("failed to install CSI: %w", err)
		}
	} else {
		log.Info("Skipping CSI installation...")
	}

	// 8. Install MetalLB
	log.Info("Installing MetalLB LoadBalancer...")
	err = m.InstallLoadBalancer()
	if err != nil {
		return fmt.Errorf("failed to install LoadBalancer: %w", err)
	}

	// 9. Download kubeconfig to local (optional step)
	var kubeconfigPath string
	if m.DownloadKubeconfig {
		log.Info("Downloading kubeconfig to local...")
		kubeconfigPath, err = m.SetupKubeconfig()
		if err != nil {
			return fmt.Errorf("failed to download kubeconfig to local: %w", err)
		}
	} else {
		// Just get the path without downloading the file
		kubeconfigPath, err = kubeconfig.GetKubeconfigPath(m.Config.Name)
		if err != nil {
			return fmt.Errorf("failed to get kubeconfig path: %w", err)
		}
		log.Info("Skipping kubeconfig download...")
	}

	log.Info("\nCluster created successfully! You can access the cluster with the following commands:")
	log.Infof("\texport KUBECONFIG=%s", kubeconfigPath)
	log.Info("\tkubectl get nodes")

	return nil
}

// CreateClusterNodes creates all cluster nodes in parallel (master and workers)
func (m *Manager) CreateClusterNodes() error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(m.Config.Workers)+1) // Create error channel for all nodes (including master)

	// Create a slice containing configurations for all nodes
	allNodes := make([]struct {
		name   string
		cpu    string
		memory string
		disk   string
	}, 0, len(m.Config.Workers)+1)

	// Add master node configuration
	allNodes = append(allNodes, struct {
		name   string
		cpu    string
		memory string
		disk   string
	}{
		name:   m.Config.Master.Name,
		cpu:    strconv.Itoa(m.Config.Master.CPU),
		memory: strconv.Itoa(m.Config.Master.Memory) + "M",
		disk:   strconv.Itoa(m.Config.Master.Disk) + "G",
	})

	// Add worker node configurations
	for i := range m.Config.Workers {
		allNodes = append(allNodes, struct {
			name   string
			cpu    string
			memory string
			disk   string
		}{
			name:   m.Config.Workers[i].Name,
			cpu:    strconv.Itoa(m.Config.Workers[i].CPU),
			memory: strconv.Itoa(m.Config.Workers[i].Memory) + "M",
			disk:   strconv.Itoa(m.Config.Workers[i].Disk) + "G",
		})
	}

	// Create all nodes in parallel
	for _, node := range allNodes {
		wg.Add(1)
		go func(nodeName, cpus, memory, disk string) {
			defer wg.Done()
			log.Infof("Creating node: %s", nodeName)
			if err := m.MultipassClient.CreateVM(nodeName, cpus, memory, disk); err != nil {
				errChan <- fmt.Errorf("failed to create node %s: %w", nodeName, err)
			}
		}(node.name, node.cpu, node.memory, node.disk)
	}

	// Wait for all node creation to complete
	wg.Wait()
	close(errChan)

	// Check if any errors occurred
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// CreateMasterNode creates master node
func (m *Manager) CreateMasterNode() error {
	masterName := m.Config.Master.Name
	cpus := strconv.Itoa(m.Config.Master.CPU)
	memory := strconv.Itoa(m.Config.Master.Memory) + "M"
	disk := strconv.Itoa(m.Config.Master.Disk) + "G"

	return m.MultipassClient.CreateVM(masterName, cpus, memory, disk)
}

// CreateWorkerNodes creates worker nodes
func (m *Manager) CreateWorkerNodes() error {
	for i := range m.Config.Workers {
		nodeName := m.Config.Workers[i].Name
		cpus := strconv.Itoa(m.Config.Workers[i].CPU)
		memory := strconv.Itoa(m.Config.Workers[i].Memory) + "M"
		disk := strconv.Itoa(m.Config.Workers[i].Disk) + "G"

		err := m.MultipassClient.CreateVM(nodeName, cpus, memory, disk)
		if err != nil {
			return fmt.Errorf("failed to create Worker node %s: %w", nodeName, err)
		}
	}
	return nil
}

func (m *Manager) InitializeEnvironment() error {
	// Collect all node names into a slice
	nodeNames := []string{m.Config.Master.Name}
	for _, worker := range m.Config.Workers {
		nodeNames = append(nodeNames, worker.Name)
	}

	// Use parallel batch initializer to support parallel initialization of multiple nodes
	batchInitializer := environment.NewParallelBatchInitializerWithOptions(m, nodeNames, m.InitOptions)

	// Use concurrency limit to execute initialization (limit to 2 nodes at a time to avoid apt lock contention)
	if err := batchInitializer.InitializeWithConcurrencyLimit(3); err != nil {
		return fmt.Errorf("failed to initialize environment: %w", err)
	}

	return nil
}

// InitializeMaster initializes master node
func (m *Manager) InitializeMaster() (string, error) {
	// Create KubeadmConfig
	m.KubeadmConfig = kubeadm.NewKubeadmConfig(m.SSHManager, m.Config.K8sVersion, m.Config.Master.Name)

	// Use new config system to generate config file and initialize Master
	err := m.KubeadmConfig.InitMaster()
	if err != nil {
		return "", fmt.Errorf("failed to initialize Master node: %w", err)
	}
	return m.getJoinCommand()
}

// JoinWorkerNodes joins worker nodes to the cluster
func (m *Manager) JoinWorkerNodes(joinCommand string) error {
	for i := range m.Config.Workers {
		err := m.JoinWorkerNode(m.Config.Workers[i].Name, joinCommand)
		if err != nil {
			return err
		}
	}
	return nil
}

// getJoinCommand gets the join command
func (m *Manager) getJoinCommand() (string, error) {
	return m.KubeadmConfig.PrintJoinCommand()
}

// JoinWorkerNode joins a worker node to the cluster
func (m *Manager) JoinWorkerNode(nodeName, joinCommand string) error {
	return m.KubeadmConfig.JoinNode(nodeName, joinCommand)
}

// InstallCNI installs CNI
func (m *Manager) InstallCNI() error {
	// Ensure master node's SSH client is created
	sshClient, exists := m.SSHManager.GetClient(m.Config.Master.Name)
	if !exists {
		return fmt.Errorf("failed to get SSH client for Master node")
	}

	switch m.CNIType {
	case "cilium":
		// Get Master node IP
		ciliumInstaller := cni.NewCiliumInstaller(sshClient, m.Config.Master.Name, m.Cluster.GetMasterIP())
		err := ciliumInstaller.Install()
		if err != nil {
			return fmt.Errorf("failed to install Cilium CNI: %w", err)
		}

	case "flannel":
		flannelInstaller := cni.NewFlannelInstaller(sshClient, m.Config.Master.Name)
		err := flannelInstaller.Install()
		if err != nil {
			return fmt.Errorf("failed to install Flannel CNI: %w", err)
		}

	default:
		return fmt.Errorf("unsupported CNI type: %s", m.CNIType)
	}

	return nil
}

// InstallCSI installs CSI
func (m *Manager) InstallCSI() error {
	// Ensure master node's SSH client is created
	sshClient, exists := m.SSHManager.GetClient(m.Config.Master.Name)
	if !exists {
		return fmt.Errorf("failed to get SSH client for Master node")
	}

	switch m.CSIType {
	case "rook-ceph":
		// Use Rook installer to install Rook-Ceph CSI
		rookInstaller := csi.NewRookInstaller(sshClient, m.Config.Master.Name)
		err := rookInstaller.Install()
		if err != nil {
			return fmt.Errorf("failed to install Rook-Ceph CSI: %w", err)
		}

	case "local-path-provisioner":
		// Use LocalPath installer to install local-path-provisioner
		localPathInstaller := csi.NewLocalPathInstaller(sshClient, m.Config.Master.Name)
		err := localPathInstaller.Install()
		if err != nil {
			return fmt.Errorf("failed to install local-path-provisioner: %w", err)
		}

	case "none":
		return nil

	default:
		return fmt.Errorf("unsupported CSI type: %s", m.CSIType)
	}

	return nil
}

// InstallLoadBalancer installs LoadBalancer (MetalLB)
func (m *Manager) InstallLoadBalancer() error {
	// Ensure master node's SSH client is created
	sshClient, exists := m.SSHManager.GetClient(m.Config.Master.Name)
	if !exists {
		return fmt.Errorf("failed to get SSH client for Master node")
	}

	// Use MetalLB installer
	metallbInstaller := lb.NewMetalLBInstaller(sshClient, m.Config.Master.Name)
	if err := metallbInstaller.Install(); err != nil {
		return fmt.Errorf("failed to install MetalLB: %w", err)
	}

	return nil
}

// AddNode adds a new node to the cluster
func (m *Manager) AddNode(role string, cpu int, memory int, disk int) error {
	// Determine node name
	nodeName := ""
	index := 0
	if role == cluster.RoleMaster {
		// Currently only support single master
		return fmt.Errorf("currently only support single Master node")
	} else {
		for _, worker := range m.Cluster.Workers {
			suffix := strings.Split(strings.TrimPrefix(worker.Name, m.Config.Prefix()), "-")[1]
			suffixInt, err := strconv.Atoi(suffix)
			if err != nil {
				return fmt.Errorf("failed to convert index for node %s: %w", worker.Name, err)
			}
			if suffixInt > index {
				index = suffixInt
			}
		}
	}

	nodeName = m.Config.GetWorkerVMName(index + 1)
	// Create virtual machine
	cpuStr := strconv.Itoa(cpu)
	memoryStr := strconv.Itoa(memory) + "M"
	diskStr := strconv.Itoa(disk) + "G"
	log.Infof("Creating node %s, cpu: %s, memory: %s, disk: %s", nodeName, cpuStr, memoryStr, diskStr)
	err := m.MultipassClient.CreateVM(nodeName, cpuStr, memoryStr, diskStr)
	if err != nil {
		return fmt.Errorf("failed to create node %s: %w", nodeName, err)
	}

	// Update node information and get IP
	log.Infof("Updating node information %s", nodeName)
	err = m.AddNodeInfo(nodeName, cpu, memory, disk)
	if err != nil {
		return fmt.Errorf("failed to update node information: %w", err)
	}
	log.Infof("Updating cluster information %s", nodeName)
	err = cluster.SaveClusterInfomation(m.Cluster)
	if err != nil {
		return fmt.Errorf("failed to update cluster information: %w", err)
	}

	// Use initializer to set environment
	log.Infof("Initializing node %s", nodeName)
	initializer := environment.NewInitializerWithOptions(m, nodeName, m.InitOptions)
	if err := initializer.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize node %s environment: %w", nodeName, err)
	}

	// Get join command
	log.Infof("Getting join command %s", nodeName)
	joinCmd, err := m.getJoinCommand()
	if err != nil {
		return fmt.Errorf("failed to get join command: %w", err)
	}
	log.Infof("Joining cluster %s", nodeName)
	err = m.JoinWorkerNode(nodeName, joinCmd)
	if err != nil {
		return fmt.Errorf("failed to join node %s to cluster: %w", nodeName, err)
	}
	return nil
}

// DeleteNode deletes a node from the cluster
func (m *Manager) DeleteNode(nodeName string, force bool) error {
	// Check if node exists
	nodeInfo := m.Cluster.GetNodeByName(nodeName)
	if nodeInfo == nil {
		return fmt.Errorf("node %s does not exist", nodeName)
	}

	if nodeInfo.Role == cluster.RoleMaster {
		return fmt.Errorf("cannot delete Master node, please delete the entire cluster first")
	}

	// Evict node from Kubernetes
	if !force {
		err := m.drainAndDeleteNode(nodeName)
		if err != nil {
			return err
		}
	}

	// Delete virtual machine
	log.Infof("Deleting node %s...", nodeName)
	err := m.MultipassClient.DeleteVM(nodeName)
	if err != nil {
		return fmt.Errorf("failed to delete node %s: %w", nodeName, err)
	}

	// Update cluster information
	log.Infof("Updating cluster information %s", nodeName)
	m.Cluster.RemoveNode(nodeName)
	if err := cluster.SaveClusterInfomation(m.Cluster); err != nil {
		return fmt.Errorf("failed to update cluster information: %w", err)
	}

	log.Infof("Node %s has been successfully deleted!", nodeName)
	return nil
}

func (m *Manager) drainAndDeleteNode(nodeName string) error {
	log.Infof("Draining node %s from Kubernetes cluster...", nodeName)
	drainCmd := fmt.Sprintf(`kubectl drain %s --ignore-daemonsets --delete-emptydir-data --force`, nodeName)
	_, err := m.RunSSHCommand(m.Config.Master.Name, drainCmd)
	if err != nil {
		return fmt.Errorf("failed to drain node %s: %w", nodeName, err)
	}

	deleteNodeCmd := fmt.Sprintf(`kubectl delete node %s`, nodeName)
	_, err = m.RunSSHCommand(m.Config.Master.Name, deleteNodeCmd)
	if err != nil {
		return fmt.Errorf("failed to delete node %s from cluster: %w", nodeName, err)
	}
	return nil
}

// DeleteCluster deletes the cluster
func (m *Manager) DeleteCluster() error {
	log.Info("Deleting Kubernetes cluster...")

	// List all virtual machines
	prefix := m.Config.Prefix()
	vms, err := m.MultipassClient.ListVMs(prefix)
	if err != nil {
		return fmt.Errorf("failed to list virtual machines: %w", err)
	}

	// Find cluster-related virtual machines and delete them
	for _, vm := range vms {
		if strings.HasPrefix(vm, prefix) {
			log.Infof("Deleting node: %s", vm)
			// Close SSH connection
			m.SSHManager.CloseClient(vm)

			err := m.MultipassClient.DeleteVM(vm)
			if err != nil {
				log.Errorf("failed to delete node %s: %v", vm, err)
			}
		}
	}

	// Delete kubeconfig
	kubeDir := filepath.Join(os.Getenv("HOME"), ".kube")
	ohmykubeConfig := filepath.Join(kubeDir, m.Config.Name+"-config")
	if _, err := os.Stat(ohmykubeConfig); err == nil {
		os.Remove(ohmykubeConfig)
	}

	// Delete cluster information file
	homeDir, _ := os.UserHomeDir()
	clusterYaml := filepath.Join(homeDir, ".ohmykube", "cluster.yaml")
	if _, err := os.Stat(clusterYaml); err == nil {
		os.Remove(clusterYaml)
	}

	log.Info("Cluster deleted successfully!")
	return nil
}

// SetKubeadmConfigPath sets the custom kubeadm config path
func (m *Manager) SetKubeadmConfigPath(configPath string) {
	// If KubeadmConfig is not initialized, do not set the custom config path
	if m.KubeadmConfig == nil {
		log.Info("KubeadmConfig is not initialized, cannot set custom config path")
		return
	}
	m.KubeadmConfig.SetCustomConfig(configPath)
}
