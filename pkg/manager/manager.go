package manager

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/cni"
	"github.com/monshunter/ohmykube/pkg/csi"
	"github.com/monshunter/ohmykube/pkg/initializer"
	"github.com/monshunter/ohmykube/pkg/kubeadm"
	"github.com/monshunter/ohmykube/pkg/kubeconfig"
	"github.com/monshunter/ohmykube/pkg/launcher"
	"github.com/monshunter/ohmykube/pkg/lb"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// Manager cluster manager
type Manager struct {
	Config             *cluster.Config
	VMProvider         launcher.Launcher
	LauncherType       launcher.LauncherType
	KubeadmConfig      *kubeadm.KubeadmConfig
	SSHManager         *ssh.SSHManager
	Cluster            *cluster.Cluster
	InitOptions        initializer.InitOptions
	CNIType            string // CNI type to use, default is flannel
	CSIType            string // CSI type to use, default is local-path-provisioner
	DownloadKubeconfig bool   // Whether to download kubeconfig to local
}

// NewManager creates a new cluster manager
func NewManager(config *cluster.Config, sshConfig *ssh.SSHConfig, cls *cluster.Cluster) (*Manager, error) {
	// Get default launcher type for platform
	launcherType := launcher.LauncherType(config.LauncherType)

	// Create the VM launcher
	vmLauncher, err := launcher.NewLauncher(
		launcherType,
		launcher.Config{
			Image:     config.Image,
			Template:  config.Template,
			Password:  sshConfig.Password,
			SSHKey:    sshConfig.GetSSHKey(),
			SSHPubKey: sshConfig.GetSSHPubKey(),
			Parallel:  config.Parallel,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create VM launcher: %w", err)
	}

	// Create cluster information object
	if cls == nil {
		cls = cluster.NewCluster(config)
	}

	manager := &Manager{
		Config:             config,
		VMProvider:         vmLauncher,
		LauncherType:       launcherType,
		SSHManager:         ssh.NewSSHManager(cls, sshConfig),
		Cluster:            cls,
		InitOptions:        initializer.DefaultInitOptions(),
		CNIType:            "flannel",                // Default to flannel
		CSIType:            "local-path-provisioner", // Default to local-path-provisioner
		DownloadKubeconfig: true,                     // Default to download kubeconfig
	}

	// Create KubeadmConfig
	manager.KubeadmConfig = kubeadm.NewKubeadmConfig(manager.SSHManager, config.KubernetesVersion,
		cls.Spec.Master.Name, config.ProxyMode)

	return manager, nil
}

// SetInitOptions sets environment initialization options
func (m *Manager) SetInitOptions(options initializer.InitOptions) {
	m.InitOptions = options
}

// GetNodeIP gets the node IP address
func (m *Manager) GetNodeIP(nodeName string) (string, error) {
	return m.VMProvider.GetNodeIP(nodeName)
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

func (m *Manager) Close() error {
	var err error
	err = m.Cluster.Save()
	if err != nil {
		return fmt.Errorf("failed to save cluster information: %w", err)
	}
	err = m.SSHManager.CloseAllClients()
	if err != nil {
		return fmt.Errorf("failed to close SSH clients: %w", err)
	}
	return nil
}

func (m *Manager) addClusterNode(nodeName string, cpu int, memory int, disk int) error {
	status, err := m.GetStatusFromRemote(nodeName)
	if err != nil {
		return fmt.Errorf("failed to get information for node %s: %w", nodeName, err)
	}
	node := cluster.NewNode(nodeName, cluster.RoleWorker, cpu, memory, disk)
	node.UpdateStatus(status)
	m.Cluster.AddNode(node)
	return nil
}

// UpdateClusterStatus updates cluster status
func (m *Manager) UpdateClusterStatus() error {
	status, err := m.GetStatusFromRemote(m.Cluster.Spec.Master.Name)
	if err != nil {
		return fmt.Errorf("failed to get Master node information: %w", err)
	}
	m.Cluster.Spec.Master.UpdateStatus(status)
	for i := range m.Cluster.Spec.Workers {
		status, err := m.GetStatusFromRemote(m.Cluster.Spec.Workers[i].Name)
		if err != nil {
			return fmt.Errorf("failed to get Worker node information: %w", err)
		}
		m.Cluster.Spec.Workers[i].UpdateStatus(status)
	}
	// Save cluster information to local file
	return m.Cluster.Save()
}

func (m *Manager) GetStatusFromRemote(nodeName string) (status cluster.NodeStatus, err error) {
	// Get IP information
	ip, err := m.GetNodeIP(nodeName)
	if err != nil {
		return status, fmt.Errorf("failed to get IP for node %s: %w", nodeName, err)
	}

	// Fill in basic information
	status.IP = ip
	status.Phase = cluster.PhaseRunning
	sshClient, err := m.SSHManager.CreateClient(nodeName, ip)
	if err != nil {
		return status, fmt.Errorf("failed to create SSH client: %w", err)
	}
	// Get hostname
	hostnameCmd := "hostname"
	hostnameOutput, err := sshClient.RunCommand(hostnameCmd)
	if err == nil {
		status.Hostname = strings.TrimSpace(hostnameOutput)
	} else {
		log.Infof("Warning: failed to get hostname for node %s: %v", nodeName, err)
		return status, fmt.Errorf("failed to get hostname for node %s: %w", nodeName, err)
	}

	// Get OS distribution information
	releaseCmd := `cat /etc/os-release | grep PRETTY_NAME | cut -d'"' -f2`
	releaseOutput, err := sshClient.RunCommand(releaseCmd)
	if err == nil {
		status.Release = strings.TrimSpace(releaseOutput)
	} else {
		log.Infof("Warning: failed to get distribution information for node %s: %v", nodeName, err)
	}

	// Get kernel version
	kernelCmd := "uname -r"
	kernelOutput, err := sshClient.RunCommand(kernelCmd)
	if err == nil {
		status.Kernel = strings.TrimSpace(kernelOutput)
	} else {
		log.Infof("Warning: failed to get kernel version for node %s: %v", nodeName, err)
	}

	// Get system architecture
	archCmd := "uname -m"
	archOutput, err := sshClient.RunCommand(archCmd)
	if err == nil {
		status.Arch = strings.TrimSpace(archOutput)
	} else {
		log.Infof("Warning: failed to get system architecture for node %s: %v", nodeName, err)
	}

	// Get OS type
	osCmd := `uname -o`
	osOutput, err := sshClient.RunCommand(osCmd)
	if err == nil {
		status.OS = strings.TrimSpace(osOutput)
	} else {
		log.Infof("Warning: failed to get OS type for node %s: %v", nodeName, err)
	}

	return status, nil
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
	sshClient, exists := m.SSHManager.GetClient(m.Cluster.GetMasterName())
	if !exists {
		return "", fmt.Errorf("failed to get SSH client for Master node")
	}

	// Use unified kubeconfig download logic
	return kubeconfig.DownloadToLocal(sshClient, m.Cluster.Name, "")
}

// CreateCluster creates a new cluster
func (m *Manager) CreateCluster() error {
	log.Info("Starting to create Kubernetes cluster...")
	// 1. Create all nodes (in parallel)
	log.Info("Creating cluster nodes...")
	err := m.CreateClusterVMs()
	if err != nil {
		return fmt.Errorf("failed to create cluster nodes: %w", err)
	}
	// 2. Initialize environment
	err = m.InitializeVMs()
	if err != nil {
		return fmt.Errorf("failed to initialize environment: %w", err)
	}
	// 3. Configure master node
	log.Info("Configuring Kubernetes Master node...")
	joinCommand, err := m.InitializeMaster()
	if err != nil {
		return fmt.Errorf("failed to initialize Master node: %w", err)
	}

	// 4. Configure worker nodes
	log.Info("Joining Worker nodes to the cluster...")
	err = m.JoinWorkerNodes(joinCommand)
	if err != nil {
		return fmt.Errorf("failed to join Worker nodes to the cluster: %w", err)
	}

	// 5. Install CNI
	if m.CNIType != "none" {
		log.Infof("Installing %s CNI...", m.CNIType)
		err = m.InstallCNI()
		if err != nil {
			return fmt.Errorf("failed to install CNI: %w", err)
		}
	} else {
		log.Info("Skipping CNI installation...")
	}

	// 6. Install CSI
	if m.CSIType != "none" {
		log.Infof("Installing %s CSI...", m.CSIType)
		err = m.InstallCSI()
		if err != nil {
			return fmt.Errorf("failed to install CSI: %w", err)
		}
	} else {
		log.Info("Skipping CSI installation...")
	}

	// 7. Install MetalLB
	log.Info("Installing MetalLB LoadBalancer...")
	err = m.InstallLoadBalancer()
	if err != nil {
		return fmt.Errorf("failed to install LoadBalancer: %w", err)
	}

	// 8. Download kubeconfig to local (optional step)
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

	log.Info("Cluster created successfully! You can access the cluster with the following commands:")
	fmt.Printf("\t\texport KUBECONFIG=%s\n", kubeconfigPath)
	fmt.Println("\t\tkubectl get nodes")

	return nil
}

// CreateClusterVMs creates all cluster VMs in parallel (master and workers)
func (m *Manager) CreateClusterVMs() error {
	var err error
	err = m.CreateMasterVM()
	if err != nil {
		return fmt.Errorf("failed to create Master node: %w", err)
	}
	err = m.CreateWorkerVMs()
	if err != nil {
		return fmt.Errorf("failed to create Worker nodes: %w", err)
	}
	err = m.UpdateClusterStatus()
	if err != nil {
		return fmt.Errorf("failed to update cluster status: %w", err)
	}
	return nil
}

// CreateMasterVM creates master node
func (m *Manager) CreateMasterVM() error {
	masterName := m.Cluster.GetMasterName()
	return m.VMProvider.CreateVM(masterName, m.Cluster.Spec.Master.Spec.CPU, m.Cluster.Spec.Master.Spec.Memory, m.Cluster.Spec.Master.Spec.Disk)
}

// CreateWorkerVMs creates worker nodes
func (m *Manager) CreateWorkerVMs() error {
	if m.Config.Parallel > 1 {
		return m.createWorkerVMsParallel()
	}
	return m.createWorkerVMs()
}

func (m *Manager) createWorkerVMs() error {
	for i := range m.Cluster.Spec.Workers {
		err := m.CreateWorkerVM(m.Cluster.Spec.Workers[i].Name, m.Cluster.Spec.Workers[i].Spec.CPU,
			m.Cluster.Spec.Workers[i].Spec.Memory, m.Cluster.Spec.Workers[i].Spec.Disk)
		if err != nil {
			return fmt.Errorf("failed to create Worker node %s: %w", m.Cluster.Spec.Workers[i].Name, err)
		}
	}
	return nil
}

func (m *Manager) createWorkerVMsParallel() error {
	var wg sync.WaitGroup
	var err error
	// concurrent by m.Config.Parallel
	sem := make(chan struct{}, m.Config.Parallel)
	errChan := make(chan error, len(m.Cluster.Spec.Workers))

	for i := range m.Cluster.Spec.Workers {
		wg.Add(1)
		go func(node *cluster.Node) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			err = m.CreateWorkerVM(node.Name, node.Spec.CPU, node.Spec.Memory, node.Spec.Disk)
			if err != nil {
				errChan <- fmt.Errorf("failed to create Worker node %s: %w", node.Name, err)
			}
		}(m.Cluster.Spec.Workers[i])
	}
	wg.Wait()
	close(sem)

	for err := range errChan {
		if err != nil {
			return err
		}
	}
	return nil
}

// CreateMasterVM creates master node
func (m *Manager) CreateWorkerVM(nodeName string, cpu int, memory int, disk int) error {
	return m.VMProvider.CreateVM(nodeName, cpu, memory, disk)
}

// CreateWorkerVMs creates worker nodes

func (m *Manager) InitializeVMs() error {
	if m.Config.Parallel > 1 {
		return m.initializeVMsParallel()
	}
	return m.initializeVMs()
}

func (m *Manager) initializeVMs() error {
	masterName := m.Cluster.GetMasterName()
	err := m.InitializeVM(masterName)
	if err != nil {
		return fmt.Errorf("failed to initialize Master node: %w", err)
	}
	for i := range m.Cluster.Spec.Workers {
		err := m.InitializeVM(m.Cluster.Spec.Workers[i].Name)
		if err != nil {
			return fmt.Errorf("failed to initialize VM %s: %w", m.Cluster.Spec.Workers[i].Name, err)
		}
	}
	return nil
}

func (m *Manager) initializeVMsParallel() error {
	var wg sync.WaitGroup
	var err error
	// concurrent by m.Config.Parallel
	sem := make(chan struct{}, m.Config.Parallel)
	errChan := make(chan error, len(m.Cluster.Spec.Workers)+1)
	nodes := append(m.Cluster.Spec.Workers, m.Cluster.Spec.Master)
	for i := range nodes {
		wg.Add(1)
		go func(node *cluster.Node) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			err = m.InitializeVM(node.Name)
			if err != nil {
				errChan <- fmt.Errorf("failed to initialize VM %s: %w", node.Name, err)
			}
		}(m.Cluster.Spec.Workers[i])
	}
	wg.Wait()
	close(sem)

	for err := range errChan {
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) InitializeVM(nodeName string) error {
	initializer := initializer.NewInitializerWithOptions(m, nodeName, m.InitOptions)
	if err := initializer.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize VM %s: %w", nodeName, err)
	}
	return nil
}

// InitializeMaster initializes master node
func (m *Manager) InitializeMaster() (string, error) {
	log.Info("Initializing Master node...")
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
		err := m.JoinWorkerNode(m.Cluster.Spec.Workers[i].Name, joinCommand)
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
	sshClient, exists := m.SSHManager.GetClient(m.Cluster.GetMasterName())
	if !exists {
		return fmt.Errorf("failed to get SSH client for Master node")
	}

	switch m.CNIType {
	case "cilium":
		// Get Master node IP
		ciliumInstaller := cni.NewCiliumInstaller(sshClient, m.Cluster.GetMasterName(), m.Cluster.GetMasterIP())
		err := ciliumInstaller.Install()
		if err != nil {
			return fmt.Errorf("failed to install Cilium CNI: %w", err)
		}

	case "flannel":
		flannelInstaller := cni.NewFlannelInstaller(sshClient, m.Cluster.GetMasterName())
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
	sshClient, exists := m.SSHManager.GetClient(m.Cluster.GetMasterName())
	if !exists {
		return fmt.Errorf("failed to get SSH client for Master node")
	}

	switch m.CSIType {
	case "rook-ceph":
		// Use Rook installer to install Rook-Ceph CSI
		rookInstaller := csi.NewRookInstaller(sshClient, m.Cluster.GetMasterName())
		err := rookInstaller.Install()
		if err != nil {
			return fmt.Errorf("failed to install Rook-Ceph CSI: %w", err)
		}

	case "local-path-provisioner":
		// Use LocalPath installer to install local-path-provisioner
		localPathInstaller := csi.NewLocalPathInstaller(sshClient, m.Cluster.GetMasterName())
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
	sshClient, exists := m.SSHManager.GetClient(m.Cluster.GetMasterName())
	if !exists {
		return fmt.Errorf("failed to get SSH client for Master node")
	}

	// Use MetalLB installer
	metallbInstaller := lb.NewMetalLBInstaller(sshClient, m.Cluster.GetMasterName())
	if err := metallbInstaller.Install(); err != nil {
		return fmt.Errorf("failed to install MetalLB: %w", err)
	}

	return nil
}

// AddWorkerNode adds a new worker node to the cluster
func (m *Manager) AddWorkerNode(cpu int, memory int, disk int) error {
	// Determine node name
	nodeName := m.Cluster.GenWorkerName()
	// Create virtual machine
	log.Infof("Creating node %s, cpu: %d, memory: %d, disk: %d", nodeName, cpu, memory, disk)
	err := m.VMProvider.CreateVM(nodeName, cpu, memory, disk)
	if err != nil {
		return fmt.Errorf("failed to create node %s: %w", nodeName, err)
	}

	// Update node information and get IP
	log.Infof("Adding node %s to cluster", nodeName)
	err = m.addClusterNode(nodeName, cpu, memory, disk)
	if err != nil {
		return fmt.Errorf("failed to add node to cluster: %w", err)
	}
	// Use initializer to set environment
	log.Infof("Initializing node %s", nodeName)
	err = m.InitializeVM(nodeName)
	if err != nil {
		return fmt.Errorf("failed to initialize node %s: %w", nodeName, err)
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

	if nodeInfo.Spec.Role == cluster.RoleMaster {
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
	err := m.VMProvider.DeleteVM(nodeName)
	if err != nil {
		return fmt.Errorf("failed to delete node %s: %w", nodeName, err)
	}

	// Update cluster information
	log.Infof("Updating cluster information %s", nodeName)
	m.Cluster.RemoveNode(nodeName)
	if err := m.Cluster.Save(); err != nil {
		return fmt.Errorf("failed to update cluster information: %w", err)
	}

	log.Infof("Node %s has been successfully deleted!", nodeName)
	return nil
}

func (m *Manager) drainAndDeleteNode(nodeName string) error {
	log.Infof("Draining node %s from Kubernetes cluster...", nodeName)
	drainCmd := fmt.Sprintf(`kubectl drain %s --ignore-daemonsets --delete-emptydir-data --force`, nodeName)
	_, err := m.RunSSHCommand(m.Cluster.GetMasterName(), drainCmd)
	if err != nil {
		return fmt.Errorf("failed to drain node %s: %w", nodeName, err)
	}

	deleteNodeCmd := fmt.Sprintf(`kubectl delete node %s`, nodeName)
	_, err = m.RunSSHCommand(m.Cluster.GetMasterName(), deleteNodeCmd)
	if err != nil {
		return fmt.Errorf("failed to delete node %s from cluster: %w", nodeName, err)
	}
	return nil
}

// DeleteCluster deletes the cluster
func (m *Manager) DeleteCluster() error {
	log.Info("Deleting Kubernetes cluster...")

	// List all virtual machines
	prefix := m.Cluster.Prefix()
	vms, err := m.VMProvider.ListVM(prefix)
	if err != nil {
		return fmt.Errorf("failed to list virtual machines: %w", err)
	}

	// Find cluster-related virtual machines and delete them
	for _, vm := range vms {
		if strings.HasPrefix(vm, prefix) {
			log.Infof("Deleting node: %s", vm)
			// Close SSH connection
			m.SSHManager.CloseClient(vm)

			err := m.VMProvider.DeleteVM(vm)
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

	// Delete cluster information
	err = cluster.RemoveCluster(m.Config.Name)
	if err != nil {
		return fmt.Errorf("failed to delete cluster information: %w", err)
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

// SetLauncherType sets the VM launcher type
func (m *Manager) SetLauncherType(launcherType launcher.LauncherType) error {
	// If the launcher type is the same, no need to change
	if m.LauncherType == launcherType {
		return nil
	}

	// Get SSH configuration
	sshConfig := m.SSHManager.GetSSHConfig()

	// Create a new launcher with the specified type
	vmLauncher, err := launcher.NewLauncher(
		launcherType,
		launcher.Config{
			Image:     m.Config.Image,
			Password:  sshConfig.Password,
			SSHKey:    sshConfig.GetSSHKey(),
			SSHPubKey: sshConfig.GetSSHPubKey(),
			Parallel:  m.Config.Parallel,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create VM launcher: %w", err)
	}

	// Update the manager with the new launcher
	m.VMProvider = vmLauncher
	m.LauncherType = launcherType
	return nil
}
