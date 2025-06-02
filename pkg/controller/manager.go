package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/monshunter/ohmykube/pkg/addons"
	"github.com/monshunter/ohmykube/pkg/addons/api"
	"github.com/monshunter/ohmykube/pkg/cache"
	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/initializer"
	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/kube"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/provider"
	"github.com/monshunter/ohmykube/pkg/provider/options"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// Manager cluster manager
type Manager struct {
	Config       *config.Config
	VMProvider   provider.Provider
	KubeManager  *kube.Manager
	sshRunner    interfaces.SSHRunner
	AddonManager addons.AddonManager
	Cluster      *config.Cluster
	InitOptions  initializer.InitOptions
}

// NewManager creates a new cluster manager
func NewManager(cfg *config.Config, sshConfig *ssh.SSHConfig, cls *config.Cluster, addonManager addons.AddonManager) (*Manager, error) {
	// Get default provider type for platform
	providerType := provider.ProviderType(cfg.Provider)

	// Create the VM provider
	vmProvider, err := provider.NewProvider(
		providerType,
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
		return nil, fmt.Errorf("failed to create VM provider: %w", err)
	}

	// Create cluster information object
	if cls == nil {
		cls = config.NewCluster(cfg)
	}

	// Set global image recorder
	cache.SetGlobalImageRecorder(cls)
	sshRunner := ssh.NewSSHManager(cls, sshConfig)
	if addonManager == nil {
		addonManager = addons.NewManager(cls, sshRunner,
			api.CNIType(cfg.CNI), api.CSIType(cfg.CSI), api.LBType(cfg.LB))
	}

	manager := &Manager{
		Config:       cfg,
		VMProvider:   vmProvider,
		sshRunner:    sshRunner,
		AddonManager: addonManager,
		Cluster:      cls,
		InitOptions:  initializer.DefaultInitOptions(),
		KubeManager: kube.NewManager(sshRunner, cfg.KubernetesVersion,
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

func (m *Manager) Close() error {
	var err error
	err = m.Cluster.Save()
	if err != nil {
		return fmt.Errorf("failed to save cluster information: %w", err)
	}
	err = m.sshRunner.Close()
	if err != nil {
		log.Warningf("failed to close SSH clients: %v", err)
	}
	return nil
}

func (m *Manager) SetStatusForNode(node *config.Node) (err error) {
	nodeName := node.Name

	// Get IP information
	ip, err := m.GetNodeIP(nodeName)
	if err != nil {
		return fmt.Errorf("failed to get IP for node %s: %w", nodeName, err)
	}

	node.SetIP(ip)
	node.SetPhase(config.PhaseRunning)

	// Get hostname
	hostnameCmd := "hostname"
	hostnameOutput, err := m.sshRunner.RunCommand(nodeName, hostnameCmd)
	if err != nil {
		log.Warningf("failed to get hostname for node %s: %v", nodeName, err)
		return fmt.Errorf("failed to get hostname for node %s: %w", nodeName, err)
	}
	node.SetHostname(strings.TrimSpace(hostnameOutput))

	// Get OS distribution information
	releaseCmd := `cat /etc/os-release | grep PRETTY_NAME | cut -d'"' -f2`
	releaseOutput, err := m.sshRunner.RunCommand(nodeName, releaseCmd)
	if err != nil {
		log.Warningf("failed to get distribution information for node %s: %v", nodeName, err)
		return err
	}
	node.SetRelease(strings.TrimSpace(releaseOutput))

	// Get kernel version
	kernelCmd := "uname -r"
	kernelOutput, err := m.sshRunner.RunCommand(nodeName, kernelCmd)
	if err != nil {
		log.Warningf("failed to get kernel version for node %s: %v", nodeName, err)
	}
	node.SetKernel(strings.TrimSpace(kernelOutput))

	// Get system architecture
	archCmd := "uname -m"
	archOutput, err := m.sshRunner.RunCommand(nodeName, archCmd)
	if err != nil {
		log.Warningf("failed to get system architecture for node %s: %v", nodeName, err)
		return err
	}
	arch := strings.TrimSpace(archOutput)
	if arch == "aarch64" {
		arch = "arm64"
	} else if arch == "x86_64" {
		arch = "amd64"
	}
	node.SetArch(arch)

	// Get OS type
	osCmd := `uname -o`
	osOutput, err := m.sshRunner.RunCommand(nodeName, osCmd)
	if err != nil {
		log.Warningf("failed to get OS type for node %s: %v", nodeName, err)
		return err
	}
	node.SetOS(strings.TrimSpace(osOutput))
	m.Cluster.Save()
	return nil
}

// RunCommand executes a command on a node via SSH (implements SSHRunner interface)
func (m *Manager) RunCommand(nodeName, command string) (string, error) {
	return m.sshRunner.RunCommand(nodeName, command)
}

// SetupKubeconfig configures local kubeconfig
func (m *Manager) SetupKubeconfig() (string, error) {
	return m.KubeManager.DownloadKubeConfig(m.Cluster.Name, "")
}

// CreateCluster creates a new cluster
func (m *Manager) CreateCluster() error {
	// Initialize multi-step progress
	progress := log.NewMultiStepProgress("Kubernetes cluster")
	progress.AddStep("vm-creation", "Creating VMs")
	progress.AddStep("environment-init", "Initializing environments")
	progress.AddStep("master-init", "Initializing master node")
	progress.AddStep("worker-join", "Joining worker nodes")
	progress.AddStep("cni-install", "Installing CNI")
	progress.AddStep("csi-install", "Installing CSI")
	if m.Config.LB == "metallb" && m.InitOptions.EnableIPVS() {
		progress.AddStep("lb-install", "Installing LoadBalancer")
	}
	progress.AddStep("kubeconfig", "Downloading kubeconfig")

	log.Debugf("Starting to create Kubernetes cluster...")

	// Check if we can resume from a previous state
	if m.canResumeClusterCreation() {
		log.Debugf("Resuming cluster creation from previous state...")
	} else {
		// Initialize cluster conditions
		m.Cluster.SetCondition(config.ConditionTypeClusterReady, config.ConditionStatusFalse,
			"ClusterCreationStarted", "Starting cluster creation process")
	}

	// 1. Create all nodes (in parallel) if not already done
	stepIndex := 0
	if !m.Cluster.HasCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue) ||
		!m.Cluster.HasAllNodeCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue) {
		progress.StartStep(stepIndex)
		log.Debugf("Creating cluster nodes...")
		err := m.CreateClusterVMs()
		if err != nil {
			progress.FailStep(stepIndex, err)
			m.Cluster.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusFalse,
				"VMCreationFailed", fmt.Sprintf("Failed to create VMs: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to create cluster nodes: %w", err)
		}
		progress.CompleteStep(stepIndex)
		m.Cluster.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue,
			"VMsCreated", "All cluster VMs created successfully")
		m.Cluster.Save()
	} else {
		log.Debugf("Skipping VM creation as it was already completed")
	}
	// 2. Initialize environment if not already done
	stepIndex++
	if !m.Cluster.HasCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue) ||
		!m.Cluster.HasAllNodeCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue) {
		progress.StartStep(stepIndex)
		log.Debugf("Initializing VM environments...")
		err := m.InitializeVMs()
		if err != nil {
			progress.FailStep(stepIndex, err)
			m.Cluster.SetCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusFalse,
				"EnvironmentInitFailed", fmt.Sprintf("Failed to initialize environments: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to initialize environment: %w", err)
		}
		progress.CompleteStep(stepIndex)
		m.Cluster.SetCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue,
			"EnvironmentsInitialized", "All VM environments initialized successfully")
		m.Cluster.Save()
	} else {
		log.Debugf("Skipping environment initialization as it was already completed")
	}

	// 3. Configure master node if not already done
	stepIndex++
	if !m.Cluster.HasCondition(config.ConditionTypeMasterInitialized, config.ConditionStatusTrue) {
		progress.StartStep(stepIndex)
		log.Debugf("Configuring Kubernetes Master node...")
		_, err := m.InitializeMaster()
		if err != nil {
			progress.FailStep(stepIndex, err)
			m.Cluster.SetCondition(config.ConditionTypeMasterInitialized, config.ConditionStatusFalse,
				"MasterInitFailed", fmt.Sprintf("Failed to initialize master: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to initialize Master node: %w", err)
		}
		progress.CompleteStep(stepIndex)
		m.Cluster.SetCondition(config.ConditionTypeMasterInitialized, config.ConditionStatusTrue,
			"MasterInitialized", "Master node initialized successfully")
		m.Cluster.Save()
	} else {
		log.Debugf("Skipping master initialization as it was already completed")
	}

	// 4. Configure worker nodes
	stepIndex++
	if !m.Cluster.HasCondition(config.ConditionTypeWorkersJoined, config.ConditionStatusTrue) ||
		!m.Cluster.HasAllNodeCondition(config.ConditionTypeJoinedCluster, config.ConditionStatusTrue) {
		progress.StartStep(stepIndex)
		log.Debugf("Joining Worker nodes to the cluster...")
		joinCommand, err := m.getJoinCommand()
		if err != nil {
			progress.FailStep(stepIndex, fmt.Errorf("failed to get join command: %w", err))
			return fmt.Errorf("failed to get join command: %w", err)
		}

		err = m.JoinWorkerNodes(joinCommand)
		if err != nil {
			progress.FailStep(stepIndex, err)
			m.Cluster.SetCondition(config.ConditionTypeWorkersJoined, config.ConditionStatusFalse,
				"WorkersJoinFailed", fmt.Sprintf("Failed to join workers: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to join Worker nodes to the cluster: %w", err)
		}
		progress.CompleteStep(stepIndex)
		m.Cluster.SetCondition(config.ConditionTypeWorkersJoined, config.ConditionStatusTrue,
			"WorkersJoined", "All worker nodes joined the cluster successfully")
		m.Cluster.Save()
	} else {
		log.Debugf("Skipping worker joining as it was already completed")
	}

	// 5. Install CNI if not already done
	stepIndex++

	if m.Config.CNI != "none" && !m.Cluster.HasCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusTrue) {
		progress.StartStep(stepIndex)
		log.Debugf("Installing %s CNI...", m.Config.CNI)
		err := m.AddonManager.InstallCNI()
		if err != nil {
			progress.FailStep(stepIndex, err)
			m.Cluster.SetCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusFalse,
				"CNIInstallFailed", fmt.Sprintf("Failed to install CNI: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to install CNI: %w", err)
		}
		progress.CompleteStep(stepIndex)
		m.Cluster.SetCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusTrue,
			"CNIInstalled", fmt.Sprintf("%s CNI installed successfully", m.Config.CNI))
		m.Cluster.Save()
	} else if m.Config.CNI == "none" {
		log.Debugf("Skipping CNI installation...")
		m.Cluster.SetCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusTrue,
			"CNISkipped", "CNI installation was skipped as configured")
		m.Cluster.Save()
	} else {
		log.Debugf("Skipping CNI installation as it was already completed")
	}

	// 6. Install CSI if not already done
	stepIndex++
	if m.Config.CSI != "none" && !m.Cluster.HasCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusTrue) {
		progress.StartStep(stepIndex)
		log.Debugf("Installing %s CSI...", m.Config.CSI)
		err := m.AddonManager.InstallCSI()
		if err != nil {
			progress.FailStep(stepIndex, err)
			m.Cluster.SetCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusFalse,
				"CSIInstallFailed", fmt.Sprintf("Failed to install CSI: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to install CSI: %w", err)
		}
		progress.CompleteStep(stepIndex)
		m.Cluster.SetCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusTrue,
			"CSIInstalled", fmt.Sprintf("%s CSI installed successfully", m.Config.CSI))
		m.Cluster.Save()
	} else if m.Config.CSI == "none" {
		log.Debugf("Skipping CSI installation...")
		m.Cluster.SetCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusTrue,
			"CSISkipped", "CSI installation was skipped as configured")
		m.Cluster.Save()
	} else {
		log.Debugf("Skipping CSI installation as it was already completed")
	}

	// 7. Install MetalLB if not already done
	if m.Config.LB == "metallb" && m.InitOptions.EnableIPVS() {
		if !m.Cluster.HasCondition(config.ConditionTypeLBInstalled, config.ConditionStatusTrue) {
			stepIndex++
			progress.AddStep("lb-install", "Installing LoadBalancer")
			progress.StartStep(stepIndex)
			log.Debugf("Installing MetalLB LoadBalancer...")
			err := m.AddonManager.InstallLB()
			if err != nil {
				progress.FailStep(stepIndex, err)
				m.Cluster.SetCondition(config.ConditionTypeLBInstalled, config.ConditionStatusFalse,
					"LoadBalancerInstallFailed", fmt.Sprintf("Failed to install LoadBalancer: %v", err))
				m.Cluster.Save()
				return fmt.Errorf("failed to install LoadBalancer: %w", err)
			}
			progress.CompleteStep(stepIndex)
			m.Cluster.SetCondition(config.ConditionTypeLBInstalled, config.ConditionStatusTrue,
				"LoadBalancerInstalled", "MetalLB LoadBalancer installed successfully")
			m.Cluster.Save()
		} else {
			log.Debugf("Skipping LoadBalancer installation as it was already completed")
		}
	}

	// Mark cluster as ready
	m.Cluster.SetCondition(config.ConditionTypeClusterReady, config.ConditionStatusTrue,
		"ClusterReady", "Kubernetes cluster is ready for use")
	m.Cluster.Status.Phase = config.ClusterPhaseRunning
	m.Cluster.Save()

	// 8. Download kubeconfig
	stepIndex++
	progress.StartStep(stepIndex)
	log.Debugf("Downloading kubeconfig to local...")
	kubeconfigPath, err := m.SetupKubeconfig()
	if err != nil {
		progress.FailStep(stepIndex, err)
		return fmt.Errorf("failed to download kubeconfig to local: %w", err)
	}
	progress.CompleteStep(stepIndex)

	// Complete the progress
	progress.Complete()

	fmt.Println()
	fmt.Println("🎉 Cluster created successfully!")
	fmt.Println()
	fmt.Println("📋 Quick Start:")
	fmt.Printf("   export KUBECONFIG=%s\n", kubeconfigPath)
	fmt.Println("   kubectl get nodes")
	fmt.Println()
	fmt.Println("🔗 Useful commands:")
	fmt.Println("   kubectl get pods --all-namespaces")
	fmt.Println("   kubectl cluster-info")
	fmt.Println()

	return nil
}

// CreateClusterVMs creates all cluster VMs in parallel (master and workers)
func (m *Manager) CreateClusterVMs() error {
	nodes := m.Cluster.Spec.Workers
	nodes = append(nodes, m.Cluster.Spec.Master)
	log.Debugf("Creating %d nodes", len(nodes))
	if m.Config.Parallel > 1 {
		return m.createVMsParallel(nodes)
	}
	return m.createVMsSequential(nodes)
}

func (m *Manager) setupVM(nodeName string, role string, cpu int, memory int, disk int) (*config.Node, error) {

	node := m.Cluster.GetNodeOrNew(nodeName, role, cpu, memory, disk)
	// log.Infof("Setting up node %s, role: %s", node.Name, role)
	nodeName = node.Name

	if node.HasCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue) {
		log.Debugf("VM %s already created, skipping creation", nodeName)
		return node, nil
	}

	// Create virtual machine
	log.Debugf("Creating node %s, cpu: %d, memory: %d, disk: %d", nodeName, cpu, memory, disk)
	node.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusFalse,
		"Creating", "Creating worker VM")

	err := m.VMProvider.Create(nodeName, cpu, memory, disk)
	if err != nil {
		node.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusFalse,
			"CreationFailed", fmt.Sprintf("Failed to create worker VM: %v", err))
		m.Cluster.Save()
		return node, fmt.Errorf("failed to create node %s: %w", nodeName, err)
	}

	node.SetCondition(config.ConditionTypeVMCreated, config.ConditionStatusTrue,
		"Created", "VM created successfully")
	m.Cluster.Save()

	err = m.SetStatusForNode(node)
	if err != nil {
		return node, fmt.Errorf("failed to get information for node %s: %w", nodeName, err)
	}
	return node, nil
}

func (m *Manager) createVMsSequential(nodes []*config.Node) error {
	for _, node := range nodes {
		_, err := m.setupVM(node.Name, node.Spec.Role,
			node.Spec.CPU, node.Spec.Memory, node.Spec.Disk)
		if err != nil {
			return fmt.Errorf("failed to create Worker node %s: %w", node.Name, err)
		}
	}
	return nil
}

func (m *Manager) createVMsParallel(nodes []*config.Node) error {
	var wg sync.WaitGroup
	var err error

	// concurrent by m.Config.Parallel
	sem := make(chan struct{}, m.Config.Parallel)
	errChan := make(chan error, len(nodes))
	wg.Add(len(nodes))
	for i := range nodes {
		sem <- struct{}{}
		go func(node *config.Node) {
			defer func() {
				wg.Done()
				<-sem
			}()
			_, err = m.setupVM(node.Name, node.Spec.Role,
				node.Spec.CPU, node.Spec.Memory, node.Spec.Disk)
			if err != nil {
				errChan <- fmt.Errorf("failed to create Worker node %s: %w", node.Name, err)
			}
		}(nodes[i])
	}
	wg.Wait()
	close(sem)
	close(errChan)

	for err := range errChan {
		if err != nil {
			return err
		}
	}
	return nil
}

// CreateWorkerVMs creates worker nodes

func (m *Manager) InitializeVMs() error {
	if m.Config.Parallel > 1 {
		return m.initializeVMsParallel()
	}
	return m.initializeVMsSequential()
}

func (m *Manager) initializeVMsSequential() error {
	masterName := m.Cluster.GetMasterName()
	err := m.initializeVM(masterName)
	if err != nil {
		return fmt.Errorf("failed to initialize Master node: %w", err)
	}
	for i := range m.Cluster.Spec.Workers {
		err := m.initializeVM(m.Cluster.Spec.Workers[i].Name)
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
	log.Debugf("Initializing %d VMs in parallel", len(nodes))
	wg.Add(len(nodes))
	for i := range nodes {
		sem <- struct{}{}
		go func(node *config.Node) {
			defer func() {
				wg.Done()
				<-sem
			}()
			err = m.initializeVM(node.Name)
			if err != nil {
				errChan <- fmt.Errorf("failed to initialize VM %s: %w", node.Name, err)
			}
		}(nodes[i])
	}
	wg.Wait()
	close(sem)
	close(errChan)
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) initializeVM(nodeName string) error {
	// Find the node in the cluster
	node := m.Cluster.GetNodeByName(nodeName)
	if node == nil {
		return fmt.Errorf("node %s not found in cluster", nodeName)
	}

	// Check if environment is already initialized
	if node.HasCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue) {
		log.Debugf("Environment for node %s already initialized, skipping initialization", nodeName)
		return nil
	}

	// Set condition to pending
	node.SetCondition(config.ConditionTypeEnvironmentInit, config.ConditionStatusFalse,
		"Initializing", "Initializing node environment")

	// Create and run the initializer
	initializer, err := initializer.NewInitializerWithOptions(m.sshRunner, node, m.InitOptions)
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
	m.Cluster.Save()

	return nil
}

// InitializeMaster initializes master node
func (m *Manager) InitializeMaster() (string, error) {
	log.Debugf("Initializing Master node...")

	// Check if master is already initialized
	if m.Cluster.Spec.Master.HasCondition(config.ConditionTypeKubeInitialized, config.ConditionStatusTrue) {
		log.Debugf("Master node already initialized, skipping initialization")
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
	m.Cluster.Save()

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
	log.Debugf("Try to Join node %s to the cluster", nodeName)
	node := m.Cluster.GetNodeByName(nodeName)
	if node == nil {
		return fmt.Errorf("node %s not found in cluster", nodeName)
	}

	// Check if node is already joined
	if node.HasCondition(config.ConditionTypeJoinedCluster, config.ConditionStatusTrue) {
		log.Debugf("Node %s already joined the cluster, skipping join", nodeName)
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

	return m.Cluster.Save()
}

func (m *Manager) AddWorkerNodes(cpu int, memory int, disk int, count int) error {
	for i := 0; i < count; i++ {
		err := m.AddWorkerNode(cpu, memory, disk)
		if err != nil {
			return err
		}
	}
	return nil
}

// AddWorkerNode adds a new worker node to the cluster
func (m *Manager) AddWorkerNode(cpu int, memory int, disk int) error {
	log.Infof("Create new worker node...")
	node, err := m.setupVM("", config.RoleWorker, cpu, memory, disk)
	if err != nil {
		return err
	}
	nodeName := node.Name

	// Use initializer to set environment
	log.Infof("Initializing node %s", nodeName)
	err = m.initializeVM(nodeName)
	if err != nil {
		return err
	}
	// Get join command
	log.Debugf("Getting join command for %s", nodeName)
	joinCmd, err := m.getJoinCommand()
	if err != nil {
		return fmt.Errorf("failed to get join command: %w", err)
	}

	log.Infof("Joining cluster with node %s", nodeName)
	node.SetCondition(config.ConditionTypeJoinedCluster, config.ConditionStatusFalse,
		"Joining", "Joining node to the cluster")

	return m.JoinWorkerNode(nodeName, joinCmd)
}

// DeleteNode deletes a node from the cluster
func (m *Manager) DeleteNode(nodeName string, force bool) error {
	log.Infof("Deleting node %s...", nodeName)
	// Check if node exists
	nodeInfo := m.Cluster.GetNodeByName(nodeName)
	if nodeInfo == nil {
		return fmt.Errorf("node %s does not exist", nodeName)
	}

	if nodeInfo.Spec.Role == config.RoleMaster && len(m.Cluster.Spec.Workers) > 0 {
		return fmt.Errorf("cannot delete Master node, please delete the entire cluster first")
	}

	// Evict node from Kubernetes
	if !force && nodeInfo.IsRunning() {
		err := m.drainAndDeleteNode(nodeName)
		if err != nil {
			return err
		}
	}

	// Delete virtual machine
	err := m.VMProvider.Delete(nodeName)
	if err != nil {
		return fmt.Errorf("failed to delete node %s: %w", nodeName, err)
	}

	// Update cluster information
	log.Debugf("Updating cluster information %s", nodeName)
	m.Cluster.RemoveNode(nodeName)
	if err := m.Cluster.Save(); err != nil {
		return fmt.Errorf("failed to update cluster information: %w", err)
	}

	fmt.Printf("🗑️  Node %s deleted successfully!\n", nodeName)
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
	log.Info("Deleting Kubernetes cluster...")

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
	fmt.Println()
	fmt.Println("🗑️  Cluster deleted successfully!")
	fmt.Println()
	return nil
}

// SetKubeadmConfigPath sets the custom kubeadm config path
func (m *Manager) SetKubeadmConfigPath(configPath string) {
	// If KubeadmConfig is not initialized, do not set the custom config path
	if m.KubeManager == nil {
		log.Warning("KubeadmConfig is not initialized, cannot set custom config path")
		return
	}
	m.KubeManager.SetCustomConfig(configPath)
}

// StartVM starts a virtual machine
func (m *Manager) StartVM(nodeName string) error {
	// Check if node exists
	log.Infof("Starting node %s...", nodeName)
	nodeInfo := m.Cluster.GetNodeByName(nodeName)
	if nodeInfo == nil {
		return fmt.Errorf("node %s does not exist", nodeName)
	}
	err := m.VMProvider.Start(nodeName)
	if err != nil {
		return fmt.Errorf("failed to start VM %s: %w", nodeName, err)
	}
	// Update cluster information
	log.Debugf("Updating cluster information %s", nodeName)
	m.Cluster.SetPhaseForNode(nodeName, config.PhaseRunning)
	if err := m.Cluster.Save(); err != nil {
		return fmt.Errorf("failed to update cluster information: %w", err)
	}
	log.Infof("VM %s started successfully", nodeName)
	return nil
}

// StopVM stops a virtual machine
func (m *Manager) StopVM(nodeName string, force bool) error {
	// Check if node exists
	log.Infof("Stopping node %s...", nodeName)
	nodeInfo := m.Cluster.GetNodeByName(nodeName)
	if nodeInfo == nil {
		return fmt.Errorf("node %s does not exist", nodeName)
	}

	// Evict node from Kubernetes
	if !force && nodeInfo.IsRunning() {
		err := m.drainAndDeleteNode(nodeName)
		if err != nil {
			return err
		}
	}
	err := m.VMProvider.Stop(nodeName)
	if err != nil {
		return fmt.Errorf("failed to stop VM %s: %w", nodeName, err)
	}
	// Update cluster information
	log.Debugf("Updating cluster information %s", nodeName)
	m.Cluster.SetPhaseForNode(nodeName, config.PhaseStopped)
	if err := m.Cluster.Save(); err != nil {
		return fmt.Errorf("failed to update cluster information: %w", err)
	}
	log.Infof("VM %s stopped successfully", nodeName)
	return nil
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
