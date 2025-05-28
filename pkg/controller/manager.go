package controller

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/monshunter/ohmykube/pkg/addons"
	"github.com/monshunter/ohmykube/pkg/addons/api"
	"github.com/monshunter/ohmykube/pkg/cache"
	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/initializer"
	"github.com/monshunter/ohmykube/pkg/kube"
	"github.com/monshunter/ohmykube/pkg/launcher"
	"github.com/monshunter/ohmykube/pkg/launcher/options"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// Manager cluster manager
type Manager struct {
	Config       *config.Config
	VMProvider   launcher.Launcher
	LauncherType launcher.LauncherType
	KubeManager  *kube.Manager
	SSHManager   *ssh.SSHManager
	AddonManager addons.AddonManager
	Cluster      *config.Cluster
	InitOptions  initializer.InitOptions
}

// NewManager creates a new cluster manager
func NewManager(cfg *config.Config, sshConfig *ssh.SSHConfig, cls *config.Cluster, addonManager addons.AddonManager) (*Manager, error) {
	// Get default launcher type for platform
	launcherType := launcher.LauncherType(cfg.LauncherType)

	// Create the VM launcher
	vmLauncher, err := launcher.NewLauncher(
		launcherType,
		&options.Options{
			Template:     cfg.Template,
			Password:     sshConfig.Password,
			SSHKey:       sshConfig.GetSSHKey(),
			SSHPubKey:    sshConfig.GetSSHPubKey(),
			Parallel:     cfg.Parallel,
			Prefix:       cfg.Name,
			Force:        true,
			OutputFormat: cfg.OutputFormat,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create VM launcher: %w", err)
	}

	// Create cluster information object
	if cls == nil {
		cls = config.NewCluster(cfg)
	}

	// Set global image recorder
	cache.SetGlobalImageRecorder(cls)
	sshManager := ssh.NewSSHManager(cls, sshConfig)
	if addonManager == nil {
		addonManager = addons.NewManager(cls, sshManager,
			api.CNIType(cfg.CNI), api.CSIType(cfg.CSI), api.LBType(cfg.LB))
	}

	manager := &Manager{
		Config:       cfg,
		VMProvider:   vmLauncher,
		LauncherType: launcherType,
		SSHManager:   sshManager,
		AddonManager: addonManager,
		Cluster:      cls,
		InitOptions:  initializer.DefaultInitOptions(),
		KubeManager: kube.NewManager(sshManager, cfg.KubernetesVersion,
			cls.Spec.Master.Name, cfg.ProxyMode),
	}

	return manager, nil
}

// SetInitOptions sets environment initialization options
func (m *Manager) SetInitOptions(options initializer.InitOptions) {
	m.InitOptions = options
}

// GetNodeIP gets the node IP address
func (m *Manager) GetNodeIP(nodeName string) (string, error) {
	return m.VMProvider.GetAddress(nodeName)
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

// UpdateClusterStatus updates cluster status
func (m *Manager) UpdateVMsInternalStatus() error {
	status, err := m.GetStatusFromRemote(m.Cluster.Spec.Master.Name)
	if err != nil {
		return fmt.Errorf("failed to get Master node information: %w", err)
	}
	m.Cluster.Spec.Master.SetInternalStatus(status)
	for i := range m.Cluster.Spec.Workers {
		status, err := m.GetStatusFromRemote(m.Cluster.Spec.Workers[i].Name)
		if err != nil {
			return fmt.Errorf("failed to get Worker node information: %w", err)
		}
		m.Cluster.Spec.Workers[i].SetInternalStatus(status)
	}
	// Save cluster information to local file
	return m.Cluster.Save()
}

func (m *Manager) GetStatusFromRemote(nodeName string) (status config.NodeInternalStatus, err error) {
	// Get IP information
	ip, err := m.GetNodeIP(nodeName)
	if err != nil {
		return status, fmt.Errorf("failed to get IP for node %s: %w", nodeName, err)
	}

	// Fill in basic information
	status.IP = ip
	status.Phase = config.PhaseRunning
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
		log.Warningf("failed to get hostname for node %s: %v", nodeName, err)
		return status, fmt.Errorf("failed to get hostname for node %s: %w", nodeName, err)
	}

	// Get OS distribution information
	releaseCmd := `cat /etc/os-release | grep PRETTY_NAME | cut -d'"' -f2`
	releaseOutput, err := sshClient.RunCommand(releaseCmd)
	if err == nil {
		status.Release = strings.TrimSpace(releaseOutput)
	} else {
		log.Warningf("failed to get distribution information for node %s: %v", nodeName, err)
	}

	// Get kernel version
	kernelCmd := "uname -r"
	kernelOutput, err := sshClient.RunCommand(kernelCmd)
	if err == nil {
		status.Kernel = strings.TrimSpace(kernelOutput)
	} else {
		log.Warningf("failed to get kernel version for node %s: %v", nodeName, err)
	}

	// Get system architecture
	archCmd := "uname -m"
	archOutput, err := sshClient.RunCommand(archCmd)
	if err == nil {
		status.Arch = strings.TrimSpace(archOutput)
		if status.Arch == "aarch64" {
			status.Arch = "arm64"
		}
	} else {
		log.Warningf("failed to get system architecture for node %s: %v", nodeName, err)
	}

	// Get OS type
	osCmd := `uname -o`
	osOutput, err := sshClient.RunCommand(osCmd)
	if err == nil {
		status.OS = strings.TrimSpace(osOutput)
	} else {
		log.Warningf("failed to get OS type for node %s: %v", nodeName, err)
	}

	return status, nil
}

// RunCommand executes a command on a node via SSH (implements SSHRunner interface)
func (m *Manager) RunCommand(nodeName, command string) (string, error) {
	return m.SSHManager.RunCommand(nodeName, command)
}

// SetupKubeconfig configures local kubeconfig
func (m *Manager) SetupKubeconfig() (string, error) {
	return m.KubeManager.DownloadKubeConfig(m.Cluster.Name, "")
}

// CreateCluster creates a new cluster
func (m *Manager) CreateCluster() error {
	log.Info("Starting to create Kubernetes config...")

	// Check if we can resume from a previous state
	if m.canResumeClusterCreation() {
		log.Info("Resuming cluster creation from previous state...")
	} else {
		// Initialize cluster conditions
		m.Cluster.SetCondition(config.ConditionTypeClusterReady, config.ConditionStatusFalse,
			"ClusterCreationStarted", "Starting cluster creation process")
	}

	// 1. Create all nodes (in parallel) if not already done
	if !m.Cluster.HasCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue) ||
		!m.Cluster.HasAllNodeCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue) {
		log.Info("Creating cluster nodes...")
		err := m.CreateClusterVMs()
		if err != nil {
			m.Cluster.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusFalse,
				"VMCreationFailed", fmt.Sprintf("Failed to create VMs: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to create cluster nodes: %w", err)
		}
		m.Cluster.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue,
			"VMsCreated", "All cluster VMs created successfully")
		m.Cluster.Save()
	} else {
		log.Info("Skipping VM creation as it was already completed")
	}
	// 2. Initialize environment if not already done
	if !m.Cluster.HasCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue) ||
		!m.Cluster.HasAllNodeCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue) {
		log.Info("Initializing VM environments...")
		err := m.InitializeVMs()
		if err != nil {
			m.Cluster.SetCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusFalse,
				"EnvironmentInitFailed", fmt.Sprintf("Failed to initialize environments: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to initialize environment: %w", err)
		}
		m.Cluster.SetCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue,
			"EnvironmentsInitialized", "All VM environments initialized successfully")
		m.Cluster.Save()
	} else {
		log.Info("Skipping environment initialization as it was already completed")
	}

	// 3. Configure master node if not already done
	if !m.Cluster.HasCondition(config.ConditionTypeMasterInitialized, config.ConditionStatusTrue) {
		log.Info("Configuring Kubernetes Master node...")
		_, err := m.InitializeMaster()
		if err != nil {
			m.Cluster.SetCondition(config.ConditionTypeMasterInitialized, config.ConditionStatusFalse,
				"MasterInitFailed", fmt.Sprintf("Failed to initialize master: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to initialize Master node: %w", err)
		}
		m.Cluster.SetCondition(config.ConditionTypeMasterInitialized, config.ConditionStatusTrue,
			"MasterInitialized", "Master node initialized successfully")
		m.Cluster.Save()
	} else {
		log.Info("Skipping master initialization as it was already completed")
	}

	// 4. Configure worker nodes
	if !m.Cluster.HasCondition(config.ConditionTypeWorkersJoined, config.ConditionStatusTrue) ||
		!m.Cluster.HasAllNodeCondition(config.ConditionTypeJoinedCluster, config.ConditionStatusTrue) {
		log.Info("Joining Worker nodes to the config...")
		joinCommand, err := m.getJoinCommand()
		if err != nil {
			return fmt.Errorf("failed to get join command: %w", err)
		}

		err = m.JoinWorkerNodes(joinCommand)
		if err != nil {
			m.Cluster.SetCondition(config.ConditionTypeWorkersJoined, config.ConditionStatusFalse,
				"WorkersJoinFailed", fmt.Sprintf("Failed to join workers: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to join Worker nodes to the cluster: %w", err)
		}
		m.Cluster.SetCondition(config.ConditionTypeWorkersJoined, config.ConditionStatusTrue,
			"WorkersJoined", "All worker nodes joined the cluster successfully")
		m.Cluster.Save()
	} else {
		log.Info("Skipping worker joining as it was already completed")
	}

	// 5. Install CNI if not already done
	if m.Config.CNI != "none" && !m.Cluster.HasCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusTrue) {
		log.Infof("Installing %s CNI...", m.Config.CNI)
		err := m.AddonManager.InstallCNI()
		if err != nil {
			m.Cluster.SetCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusFalse,
				"CNIInstallFailed", fmt.Sprintf("Failed to install CNI: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to install CNI: %w", err)
		}
		m.Cluster.SetCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusTrue,
			"CNIInstalled", fmt.Sprintf("%s CNI installed successfully", m.Config.CNI))
		m.Cluster.Save()
	} else if m.Config.CNI == "none" {
		log.Info("Skipping CNI installation...")
		m.Cluster.SetCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusTrue,
			"CNISkipped", "CNI installation was skipped as configured")
		m.Cluster.Save()
	} else {
		log.Info("Skipping CNI installation as it was already completed")
	}

	// 6. Install CSI if not already done
	if m.Config.CSI != "none" && !m.Cluster.HasCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusTrue) {
		log.Infof("Installing %s CSI...", m.Config.CSI)
		err := m.AddonManager.InstallCSI()
		if err != nil {
			m.Cluster.SetCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusFalse,
				"CSIInstallFailed", fmt.Sprintf("Failed to install CSI: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to install CSI: %w", err)
		}
		m.Cluster.SetCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusTrue,
			"CSIInstalled", fmt.Sprintf("%s CSI installed successfully", m.Config.CSI))
		m.Cluster.Save()
	} else if m.Config.CSI == "none" {
		log.Info("Skipping CSI installation...")
		m.Cluster.SetCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusTrue,
			"CSISkipped", "CSI installation was skipped as configured")
		m.Cluster.Save()
	} else {
		log.Info("Skipping CSI installation as it was already completed")
	}

	// 7. Install MetalLB if not already done
	if !m.Cluster.HasCondition(config.ConditionTypeLBInstalled, config.ConditionStatusTrue) {
		log.Info("Installing MetalLB LoadBalancer...")
		err := m.AddonManager.InstallLB()
		if err != nil {
			m.Cluster.SetCondition(config.ConditionTypeLBInstalled, config.ConditionStatusFalse,
				"LoadBalancerInstallFailed", fmt.Sprintf("Failed to install LoadBalancer: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to install LoadBalancer: %w", err)
		}
		m.Cluster.SetCondition(config.ConditionTypeLBInstalled, config.ConditionStatusTrue,
			"LoadBalancerInstalled", "MetalLB LoadBalancer installed successfully")
		m.Cluster.Save()
	} else {
		log.Info("Skipping LoadBalancer installation as it was already completed")
	}

	log.Info("Downloading kubeconfig to local...")
	kubeconfigPath, err := m.SetupKubeconfig()
	if err != nil {
		return fmt.Errorf("failed to download kubeconfig to local: %w", err)
	}

	// Mark cluster as ready
	m.Cluster.SetCondition(config.ConditionTypeClusterReady, config.ConditionStatusTrue,
		"ClusterReady", "Kubernetes cluster is ready for use")
	m.Cluster.Status.Phase = config.ClusterPhaseRunning
	m.Cluster.Save()

	log.Info("Cluster created successfully! You can access the cluster with the following commands:")
	log.Infof("\texport KUBECONFIG=%s", kubeconfigPath)
	log.Info("\tkubectl get nodes")

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
	err = m.UpdateVMsInternalStatus()
	if err != nil {
		return fmt.Errorf("failed to update cluster status: %w", err)
	}
	return nil
}

// CreateMasterVM creates master node
func (m *Manager) CreateMasterVM() error {
	masterName := m.Cluster.GetMasterName()

	// Check if VM is already created
	if m.Cluster.Spec.Master.HasCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue) {
		log.Infof("Master VM %s already created, skipping creation", masterName)
		return nil
	}

	// Set condition to pending
	m.Cluster.Spec.Master.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusFalse,
		"Creating", "Creating master VM")

	err := m.VMProvider.Create(masterName, m.Cluster.Spec.Master.Spec.CPU,
		m.Cluster.Spec.Master.Spec.Memory, m.Cluster.Spec.Master.Spec.Disk)

	if err != nil {
		m.Cluster.Spec.Master.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusFalse,
			"CreationFailed", fmt.Sprintf("Failed to create master VM: %v", err))
		return err
	}

	// Set condition to success
	m.Cluster.Spec.Master.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue,
		"Created", "Master VM created successfully")

	return nil
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
		go func(node *config.Node) {
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

// CreateWorkerVM creates a worker node
func (m *Manager) CreateWorkerVM(nodeName string, cpu int, memory int, disk int) error {
	// Find the node in the cluster
	node := m.Cluster.GetNodeByName(nodeName)
	if node == nil {
		return fmt.Errorf("node %s not found in cluster", nodeName)
	}

	// Check if VM is already created
	if node.HasCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue) {
		log.Infof("Worker VM %s already created, skipping creation", nodeName)
		return nil
	}

	// Set condition to pending
	node.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusFalse,
		"Creating", "Creating worker VM")

	err := m.VMProvider.Create(nodeName, cpu, memory, disk)

	if err != nil {
		node.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusFalse,
			"CreationFailed", fmt.Sprintf("Failed to create worker VM: %v", err))
		return err
	}

	// Set condition to success
	node.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue,
		"Created", "Worker VM created successfully")

	return nil
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
		go func(node *config.Node) {
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
	// Find the node in the cluster
	node := m.Cluster.GetNodeByName(nodeName)
	if node == nil {
		return fmt.Errorf("node %s not found in cluster", nodeName)
	}

	// Check if environment is already initialized
	if node.HasCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue) {
		log.Infof("Environment for node %s already initialized, skipping initialization", nodeName)
		return nil
	}

	// Set condition to pending
	node.SetCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusFalse,
		"Initializing", "Initializing node environment")

	// Create and run the initializer
	initializer, err := initializer.NewInitializerWithOptions(m.SSHManager, nodeName, m.InitOptions)
	if err != nil {
		node.SetCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusFalse,
			"InitializationFailed", fmt.Sprintf("Failed to create initializer: %v", err))
		return fmt.Errorf("failed to create initializer for node %s: %w", nodeName, err)
	}

	if err := initializer.Initialize(); err != nil {
		node.SetCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusFalse,
			"InitializationFailed", fmt.Sprintf("Failed to initialize environment: %v", err))
		return fmt.Errorf("failed to initialize VM %s: %w", nodeName, err)
	}

	// Set condition to success
	node.SetCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue,
		"Initialized", "Node environment initialized successfully")

	return nil
}

// InitializeMaster initializes master node
func (m *Manager) InitializeMaster() (string, error) {
	log.Info("Initializing Master node...")

	// Check if master is already initialized
	if m.Cluster.Spec.Master.HasCondition(config.ConditionTypeKubeInitialized, config.ConditionStatusTrue) {
		log.Info("Master node already initialized, skipping initialization")
		return m.getJoinCommand()
	}

	// Set condition to pending
	m.Cluster.Spec.Master.SetCondition(config.ConditionTypeKubeInitialized, config.ConditionStatusFalse,
		"Initializing", "Initializing Kubernetes master node")

	// Use new config system to generate config file and initialize Master
	err := m.KubeManager.InitMaster()
	if err != nil {
		m.Cluster.Spec.Master.SetCondition(config.ConditionTypeKubeInitialized, config.ConditionStatusFalse,
			"InitializationFailed", fmt.Sprintf("Failed to initialize master: %v", err))
		return "", fmt.Errorf("failed to initialize Master node: %w", err)
	}

	// Set condition to success
	m.Cluster.Spec.Master.SetCondition(config.ConditionTypeKubeInitialized, config.ConditionStatusTrue,
		"Initialized", "Master node initialized successfully")
	m.Cluster.Spec.Master.SetCondition(config.ConditionTypeNodeReady, config.ConditionStatusTrue,
		"NodeReady", "Master node is ready")

	return m.getJoinCommand()
}

// JoinWorkerNodes joins worker nodes to the cluster
func (m *Manager) JoinWorkerNodes(joinCommand string) error {
	for i := range m.Cluster.Spec.Workers {
		err := m.JoinWorkerNode(m.Cluster.Spec.Workers[i].Name, joinCommand)
		if err != nil {
			return err
		}
	}
	return nil
}

// getJoinCommand gets the join command
func (m *Manager) getJoinCommand() (string, error) {
	return m.KubeManager.PrintJoinCommand()
}

// JoinWorkerNode joins a worker node to the cluster
func (m *Manager) JoinWorkerNode(nodeName, joinCommand string) error {
	// Find the node in the cluster
	log.Infof("Try to Join node %s to the cluster", nodeName)
	node := m.Cluster.GetNodeByName(nodeName)
	if node == nil {
		return fmt.Errorf("node %s not found in cluster", nodeName)
	}

	// Check if node is already joined
	if node.HasCondition(config.ConditionTypeJoinedCluster, config.ConditionStatusTrue) {
		log.Infof("Node %s already joined the cluster, skipping join", nodeName)
		return nil
	}

	// Set condition to pending
	node.SetCondition(config.ConditionTypeJoinedCluster, config.ConditionStatusFalse,
		"Joining", "Joining node to the cluster")

	// Join the node to the cluster
	err := m.KubeManager.JoinNode(nodeName, joinCommand)
	if err != nil {
		node.SetCondition(config.ConditionTypeJoinedCluster, config.ConditionStatusFalse,
			"JoinFailed", fmt.Sprintf("Failed to join node to cluster: %v", err))
		return err
	}

	// Set condition to success
	node.SetCondition(config.ConditionTypeJoinedCluster, config.ConditionStatusTrue,
		"Joined", "Node joined the cluster successfully")
	node.SetCondition(config.ConditionTypeNodeReady, config.ConditionStatusTrue,
		"NodeReady", "Worker node is ready")

	return nil
}

// AddWorkerNode adds a new worker node to the cluster
func (m *Manager) AddWorkerNode(cpu int, memory int, disk int) error {
	// Determine node name
	nodeName := m.Cluster.GenWorkerName()

	// Create a new node object
	node := config.NewNode(nodeName, config.RoleWorker, cpu, memory, disk)

	// Add the node to the cluster
	m.Cluster.AddNode(node)

	// Create virtual machine
	log.Infof("Creating node %s, cpu: %d, memory: %d, disk: %d", nodeName, cpu, memory, disk)
	node.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusFalse,
		"Creating", "Creating worker VM")

	err := m.VMProvider.Create(nodeName, cpu, memory, disk)
	if err != nil {
		node.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusFalse,
			"CreationFailed", fmt.Sprintf("Failed to create worker VM: %v", err))
		m.Cluster.Save()
		return fmt.Errorf("failed to create node %s: %w", nodeName, err)
	}

	node.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue,
		"Created", "Worker VM created successfully")
	m.Cluster.Save()

	// Update node information and get IP
	log.Infof("Adding node %s to cluster", nodeName)
	status, err := m.GetStatusFromRemote(nodeName)
	if err != nil {
		return fmt.Errorf("failed to get information for node %s: %w", nodeName, err)
	}
	node.SetInternalStatus(status)
	m.Cluster.Save()

	// Use initializer to set environment
	log.Infof("Initializing node %s", nodeName)
	node.SetCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusFalse,
		"Initializing", "Initializing node environment")

	err = m.InitializeVM(nodeName)
	if err != nil {
		node.SetCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusFalse,
			"InitializationFailed", fmt.Sprintf("Failed to initialize environment: %v", err))
		m.Cluster.Save()
		return fmt.Errorf("failed to initialize node %s: %w", nodeName, err)
	}

	node.SetCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue,
		"Initialized", "Node environment initialized successfully")
	m.Cluster.Save()
	// Get join command
	log.Infof("Getting join command for %s", nodeName)
	joinCmd, err := m.getJoinCommand()
	if err != nil {
		return fmt.Errorf("failed to get join command: %w", err)
	}

	log.Infof("Joining cluster with node %s", nodeName)
	node.SetCondition(config.ConditionTypeJoinedCluster, config.ConditionStatusFalse,
		"Joining", "Joining node to the cluster")

	err = m.JoinWorkerNode(nodeName, joinCmd)
	if err != nil {
		node.SetCondition(config.ConditionTypeJoinedCluster, config.ConditionStatusFalse,
			"JoinFailed", fmt.Sprintf("Failed to join node to cluster: %v", err))
		m.Cluster.Save()
		return fmt.Errorf("failed to join node %s to cluster: %w", nodeName, err)
	}

	node.SetCondition(config.ConditionTypeJoinedCluster, config.ConditionStatusTrue,
		"Joined", "Node joined the cluster successfully")
	node.SetCondition(config.ConditionTypeNodeReady, config.ConditionStatusTrue,
		"NodeReady", "Worker node is ready")
	m.Cluster.Save()

	return nil
}

// DeleteNode deletes a node from the cluster
func (m *Manager) DeleteNode(nodeName string, force bool) error {
	// Check if node exists
	nodeInfo := m.Cluster.GetNodeByName(nodeName)
	if nodeInfo == nil {
		return fmt.Errorf("node %s does not exist", nodeName)
	}

	if nodeInfo.Spec.Role == config.RoleMaster && len(m.Cluster.Spec.Workers) > 0 {
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
	err := m.VMProvider.Delete(nodeName)
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
	log.Infof("Draining node %s from Kubernetes...", nodeName)
	drainCmd := fmt.Sprintf(`kubectl drain %s --ignore-daemonsets --delete-emptydir-data --force`, nodeName)
	_, err := m.RunCommand(m.Cluster.GetMasterName(), drainCmd)
	if err != nil {
		return fmt.Errorf("failed to drain node %s: %w", nodeName, err)
	}

	deleteNodeCmd := fmt.Sprintf(`kubectl delete node %s`, nodeName)
	_, err = m.RunCommand(m.Cluster.GetMasterName(), deleteNodeCmd)
	if err != nil {
		return fmt.Errorf("failed to delete node %s from cluster: %w", nodeName, err)
	}
	return nil
}

// DeleteCluster deletes the cluster
func (m *Manager) DeleteCluster() error {
	log.Info("Deleting Kubernetes config...")

	// List all virtual machines
	prefix := m.Cluster.Prefix()
	vms, err := m.VMProvider.List()
	if err != nil {
		return fmt.Errorf("failed to list virtual machines: %w", err)
	}

	// Find cluster-related virtual machines and delete them
	for _, vm := range vms {
		if strings.HasPrefix(vm, prefix) {
			log.Infof("Deleting node: %s", vm)
			// Close SSH connection
			m.SSHManager.CloseClient(vm)

			err := m.VMProvider.Delete(vm)
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
	err = config.RemoveCluster(m.Config.Name)
	if err != nil {
		return fmt.Errorf("failed to delete cluster information: %w", err)
	}

	// Mark cluster as deleted
	m.Cluster.MarkDeleted()
	log.Info("Cluster deleted successfully!")
	return nil
}

// SetKubeadmConfigPath sets the custom kubeadm config path
func (m *Manager) SetKubeadmConfigPath(configPath string) {
	// If KubeadmConfig is not initialized, do not set the custom config path
	if m.KubeManager == nil {
		log.Info("KubeadmConfig is not initialized, cannot set custom config path")
		return
	}
	m.KubeManager.SetCustomConfig(configPath)
}

// StartVM starts a virtual machine
func (m *Manager) StartVM(nodeName string) error {
	// Check if node exists
	nodeInfo := m.Cluster.GetNodeByName(nodeName)
	if nodeInfo == nil {
		return fmt.Errorf("node %s does not exist", nodeName)
	}
	return m.VMProvider.Start(nodeName)
}

// StopVM stops a virtual machine
func (m *Manager) StopVM(nodeName string, force bool) error {
	// Check if node exists
	nodeInfo := m.Cluster.GetNodeByName(nodeName)
	if nodeInfo == nil {
		return fmt.Errorf("node %s does not exist", nodeName)
	}

	// Evict node from Kubernetes
	if !force {
		err := m.drainAndDeleteNode(nodeName)
		if err != nil {
			return err
		}
	}
	return m.VMProvider.Stop(nodeName)
}

// ListVMs lists all virtual machines with optional format
func (m *Manager) ListVMs() ([]string, error) {
	return m.VMProvider.List()
}

// OpenShell opens an interactive shell to a virtual machine
func (m *Manager) OpenShell(nodeName string) error {
	// Check if node exists
	nodeInfo := m.Cluster.GetNodeByName(nodeName)
	if nodeInfo == nil {
		return fmt.Errorf("node %s does not exist", nodeName)
	}
	return m.VMProvider.Shell(nodeName)
}

// canResumeClusterCreation checks if there are any conditions set that indicate
// a previous cluster creation attempt that can be resumed
func (m *Manager) canResumeClusterCreation() bool {
	// Check if any workflow conditions are set to true
	if m.Cluster.HasCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue) ||
		m.Cluster.HasCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue) ||
		m.Cluster.HasCondition(config.ConditionTypeMasterInitialized, config.ConditionStatusTrue) ||
		m.Cluster.HasCondition(config.ConditionTypeWorkersJoined, config.ConditionStatusTrue) ||
		m.Cluster.HasCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusTrue) ||
		m.Cluster.HasCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusTrue) ||
		m.Cluster.HasCondition(config.ConditionTypeLBInstalled, config.ConditionStatusTrue) {
		return true
	}

	// Check if any node has workflow conditions set
	if m.Cluster.Spec.Master != nil {
		for _, cond := range m.Cluster.Spec.Master.Status.Conditions {
			if (cond.Type == config.ConditionTypeVMCreated ||
				cond.Type == config.ConditionTypeEnvironmentInit ||
				cond.Type == config.ConditionTypeKubeInitialized ||
				cond.Type == config.ConditionTypeJoinedCluster) &&
				cond.Status == config.ConditionStatusTrue {
				return true
			}
		}
	}

	for _, worker := range m.Cluster.Spec.Workers {
		if worker != nil {
			for _, cond := range worker.Status.Conditions {
				if (cond.Type == config.ConditionTypeVMCreated ||
					cond.Type == config.ConditionTypeEnvironmentInit ||
					cond.Type == config.ConditionTypeJoinedCluster) &&
					cond.Status == config.ConditionStatusTrue {
					return true
				}
			}
		}
	}

	return false
}
