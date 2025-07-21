package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	closed       uint32
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
			cls.GetMasterName(), cls.GetProxyMode()),
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
	if atomic.CompareAndSwapUint32(&m.closed, 0, 1) {
		var err error
		err = m.Cluster.Save()
		if err != nil {
			return fmt.Errorf("failed to save cluster information: %w", err)
		}
		err = m.sshRunner.Close()
		if err != nil {
			log.Warningf("failed to close SSH clients: %v", err)
		}
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
	progress.AddStep("vm-creation", "Preparing virtual machines")
	progress.AddStep("auth-init", "Initializing authentication")
	progress.AddStep("environment-init", "Initializing node environments")
	progress.AddStep("master-init", "Starting contol-plane")
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

	// 2. Initialize authentication if not already done
	stepIndex++
	if !m.Cluster.HasCondition(config.ConditionTypeAuthInitialized, config.ConditionStatusTrue) ||
		!m.Cluster.HasAllNodeCondition(config.ConditionTypeAuthInitialized, config.ConditionStatusTrue) {
		progress.StartStep(stepIndex)
		log.Debugf("Initializing authentication for VMs...")
		err := m.InitializeAuth()
		if err != nil {
			progress.FailStep(stepIndex, err)
			m.Cluster.SetCondition(config.ConditionTypeAuthInitialized, config.ConditionStatusFalse,
				"AuthInitFailed", fmt.Sprintf("Failed to initialize authentication: %v", err))
			m.Cluster.Save()
			return fmt.Errorf("failed to initialize authentication: %w", err)
		}
		progress.CompleteStep(stepIndex)
		m.Cluster.SetCondition(config.ConditionTypeAuthInitialized, config.ConditionStatusTrue,
			"AuthInitialized", "All VM authentication initialized successfully")
		m.Cluster.Save()
	} else {
		log.Debugf("Skipping authentication initialization as it was already completed")
	}

	// 3. Initialize environment if not already done
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

	// 4. Execute worker join, CNI, CSI, and LB installation in parallel
	stepIndex++
	err := m.executeParallelSteps(progress, stepIndex)
	if err != nil {
		return fmt.Errorf("failed to execute parallel steps: %w", err)
	}

	// Mark cluster as ready
	m.Cluster.SetCondition(config.ConditionTypeClusterReady, config.ConditionStatusTrue,
		"ClusterReady", "Kubernetes cluster is ready for use")
	m.Cluster.Status.Phase = config.ClusterPhaseRunning
	m.Cluster.Save()

	// 8. Download kubeconfig - calculate the correct step index
	// Steps: vm-creation(0), environment-init(1), master-init(2), worker-join(3), cni-install(4), csi-install(5), [lb-install(6)], kubeconfig(last)
	kubeconfigStepIndex := 6 // Base index for kubeconfig
	if m.Config.LB == "metallb" && m.InitOptions.EnableIPVS() {
		kubeconfigStepIndex = 7 // If LB step exists, kubeconfig is at index 7
	}

	progress.StartStep(kubeconfigStepIndex)
	log.Debugf("Downloading kubeconfig to local...")
	kubeconfigPath, err := m.SetupKubeconfig()
	if err != nil {
		progress.FailStep(kubeconfigStepIndex, err)
		return fmt.Errorf("failed to download kubeconfig to local: %w", err)
	}
	progress.CompleteStep(kubeconfigStepIndex)

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
	// Sync node groups from spec without losing existing status
	m.syncNodeGroupsFromSpec()

	// Create VMs for all node groups
	totalNodes := m.Cluster.GetTotalDesiredNodes()
	log.Debugf("Creating %d nodes", totalNodes)

	if m.Config.Parallel > 1 {
		return m.createVMsParallelFromGroups()
	}
	return m.createVMsSequentialFromGroups()
}

// syncNodeGroupsFromSpec syncs node groups from spec while preserving existing status
func (m *Manager) syncNodeGroupsFromSpec() {
	// For each spec group, ensure corresponding status group exists with correct desire count
	for _, masterSpec := range m.Cluster.Spec.Nodes.Master {
		statusGroup := m.Cluster.GetOrCreateNodeGroup(masterSpec.GroupID)
		statusGroup.Desire = masterSpec.Replica
	}

	for _, workerSpec := range m.Cluster.Spec.Nodes.Workers {
		statusGroup := m.Cluster.GetOrCreateNodeGroup(workerSpec.GroupID)
		statusGroup.Desire = workerSpec.Replica
	}
}

func (m *Manager) setupVM(nodeName string, role string, groupID int, cpu int, memory int, disk int) (*config.NodeGroupMember, error) {

	// Get or create node in the appropriate group
	node := m.Cluster.GetNodeByName(nodeName)
	if node == nil {
		if nodeName == "" {
			nodeName = m.Cluster.GenNodeName(role)
		}
		node = m.Cluster.CreateNodeInGroup(groupID, nodeName)
	}

	if m.Cluster.HasNodeCondition(nodeName, config.ConditionTypeVMCreated, config.ConditionStatusTrue) {
		log.Debugf("VM %s already created, skipping creation", nodeName)
		return node, nil
	}

	// Create virtual machine
	log.Debugf("Creating node %s, cpu: %d, memory: %d, disk: %d", nodeName, cpu, memory, disk)
	m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeVMCreated, config.ConditionStatusFalse,
		"Creating", "Creating VM")

	err := m.VMProvider.Create(nodeName, cpu, memory, disk)
	if err != nil {
		m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeVMCreated, config.ConditionStatusFalse,
			"CreationFailed", fmt.Sprintf("Failed to create VM: %v", err))
		m.Cluster.Save()
		return node, fmt.Errorf("failed to create node %s: %w", nodeName, err)
	}

	m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeVMCreated, config.ConditionStatusTrue,
		"Created", "VM created successfully")
	m.Cluster.Save()
	return node, nil
}

// SetStatusForNodeByName sets status information for a node by name
func (m *Manager) SetStatusForNodeByName(nodeName string) error {
	// Get IP information
	ip, err := m.GetNodeIP(nodeName)
	if err != nil {
		return fmt.Errorf("failed to get IP for node %s: %w", nodeName, err)
	}
	m.Cluster.SetNodeIP(nodeName, ip)
	m.Cluster.SetPhaseForNode(nodeName, config.PhaseRunning)

	// Ensure VM is running before establishing SSH connection
	// This is especially important in resume mode where VMs may have been stopped
	vmStatus, err := m.VMProvider.Status(nodeName)
	if err != nil {
		return fmt.Errorf("failed to get VM status for node %s: %w", nodeName, err)
	}

	if !vmStatus.IsRunning() {
		log.Infof("VM %s is not running, starting it...", nodeName)
		err := m.VMProvider.Start(nodeName)
		if err != nil {
			return fmt.Errorf("failed to start VM %s: %w", nodeName, err)
		}
		log.Infof("VM %s started successfully", nodeName)
	}

	// Wait for SSH service to be ready with retry
	log.Debugf("Waiting for SSH service to be ready on %s...", nodeName)
	err = m.waitForSSHReady(nodeName, ip, 60*time.Second)
	if err != nil {
		return fmt.Errorf("SSH service not ready on node %s after VM start: %w", nodeName, err)
	}

	// Get hostname
	hostnameCmd := "hostname"
	hostnameOutput, err := m.sshRunner.RunCommand(nodeName, hostnameCmd)
	if err != nil {
		log.Warningf("failed to get hostname for node %s: %v", nodeName, err)
		return fmt.Errorf("failed to get hostname for node %s: %w", nodeName, err)
	}
	m.Cluster.SetNodeHostname(nodeName, strings.TrimSpace(hostnameOutput))

	// Get OS distribution information
	releaseCmd := `cat /etc/os-release | grep PRETTY_NAME | cut -d'"' -f2`
	releaseOutput, err := m.sshRunner.RunCommand(nodeName, releaseCmd)
	if err != nil {
		log.Warningf("failed to get distribution information for node %s: %v", nodeName, err)
		return err
	}

	// Get kernel version
	kernelCmd := "uname -r"
	kernelOutput, err := m.sshRunner.RunCommand(nodeName, kernelCmd)
	if err != nil {
		log.Warningf("failed to get kernel version for node %s: %v", nodeName, err)
	}

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

	// Get OS type
	osCmd := `uname -o`
	osOutput, err := m.sshRunner.RunCommand(nodeName, osCmd)
	if err != nil {
		log.Warningf("failed to get OS type for node %s: %v", nodeName, err)
		return err
	}

	// Update all system information
	m.Cluster.SetNodeSystemInfo(nodeName, strings.TrimSpace(releaseOutput), strings.TrimSpace(kernelOutput), arch, strings.TrimSpace(osOutput))
	m.Cluster.Save()
	return nil
}

// createVMsSequentialFromGroups creates VMs sequentially from node groups
func (m *Manager) createVMsSequentialFromGroups() error {
	// Create master nodes (only missing ones)
	for _, masterSpec := range m.Cluster.Spec.Nodes.Master {
		statusGroup := m.Cluster.GetNodeGroupByID(masterSpec.GroupID)
		if statusGroup == nil {
			continue
		}

		// Calculate how many nodes we need to create
		needed := statusGroup.Desire - len(statusGroup.Members)
		for i := 0; i < needed; i++ {
			nodeName := m.Cluster.GenNodeName(config.RoleMaster)
			cpu, memory, disk := m.parseResources(masterSpec.Resources)
			_, err := m.setupVM(nodeName, config.RoleMaster, masterSpec.GroupID, cpu, memory, disk)
			if err != nil {
				return err
			}
		}
	}

	// Create worker nodes (only missing ones)
	for _, workerSpec := range m.Cluster.Spec.Nodes.Workers {
		statusGroup := m.Cluster.GetNodeGroupByID(workerSpec.GroupID)
		if statusGroup == nil {
			continue
		}

		// Calculate how many nodes we need to create
		needed := statusGroup.Desire - len(statusGroup.Members)
		for i := 0; i < needed; i++ {
			nodeName := m.Cluster.GenNodeName(config.RoleWorker)
			cpu, memory, disk := m.parseResources(workerSpec.Resources)
			_, err := m.setupVM(nodeName, config.RoleWorker, workerSpec.GroupID, cpu, memory, disk)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// createVMsParallelFromGroups creates VMs in parallel from node groups
func (m *Manager) createVMsParallelFromGroups() error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, m.Config.Parallel)
	errChan := make(chan error, m.Cluster.GetTotalDesiredNodes())

	// Create master nodes (only missing ones)
	for _, masterSpec := range m.Cluster.Spec.Nodes.Master {
		statusGroup := m.Cluster.GetNodeGroupByID(masterSpec.GroupID)
		if statusGroup == nil {
			continue
		}

		// Calculate how many nodes we need to create
		needed := statusGroup.Desire - len(statusGroup.Members)
		for i := 0; i < needed; i++ {
			wg.Add(1)
			go func(spec config.NodeGroupSpec) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				nodeName := m.Cluster.GenNodeName(config.RoleMaster)
				cpu, memory, disk := m.parseResources(spec.Resources)
				_, err := m.setupVM(nodeName, config.RoleMaster, spec.GroupID, cpu, memory, disk)
				if err != nil {
					errChan <- err
				}
			}(masterSpec)
		}
	}

	// Create worker nodes (only missing ones)
	for _, workerSpec := range m.Cluster.Spec.Nodes.Workers {
		statusGroup := m.Cluster.GetNodeGroupByID(workerSpec.GroupID)
		if statusGroup == nil {
			continue
		}

		// Calculate how many nodes we need to create
		needed := statusGroup.Desire - len(statusGroup.Members)
		for i := 0; i < needed; i++ {
			wg.Add(1)
			go func(spec config.NodeGroupSpec) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				nodeName := m.Cluster.GenNodeName(config.RoleWorker)
				cpu, memory, disk := m.parseResources(spec.Resources)
				_, err := m.setupVM(nodeName, config.RoleWorker, spec.GroupID, cpu, memory, disk)
				if err != nil {
					errChan <- err
				}
			}(workerSpec)
		}
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// parseResources parses resource requirements from string format to integers
func (m *Manager) parseResources(resources config.ResourceRequests) (cpu, memory, disk int) {
	// Parse CPU (remove any suffix and convert to int)
	cpu = 1 // default
	if resources.CPU != "" {
		if parsed, err := strconv.Atoi(resources.CPU); err == nil {
			cpu = parsed
		}
	}

	// Parse Memory (remove "Gi" suffix and convert to int)
	memory = 2 // default
	if resources.Memory != "" {
		memStr := strings.TrimSuffix(resources.Memory, "Gi")
		if parsed, err := strconv.Atoi(memStr); err == nil {
			memory = parsed
		}
	}

	// Parse Storage (remove "Gi" suffix and convert to int)
	disk = 10 // default
	if resources.Storage != "" {
		diskStr := strings.TrimSuffix(resources.Storage, "Gi")
		if parsed, err := strconv.Atoi(diskStr); err == nil {
			disk = parsed
		}
	}

	return cpu, memory, disk
}

// waitForSSHReady waits for SSH service to be ready on a node
func (m *Manager) waitForSSHReady(nodeName, ip string, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		if sshManager, ok := m.sshRunner.(*ssh.SSHManager); ok {
			client, err := sshManager.CreateClient(nodeName, ip)
			if err == nil && client != nil {
				// Test the connection with a simple command
				_, err := client.RunCommand("echo 'SSH ready'")
				if err == nil {
					log.Debugf("SSH service ready on node %s", nodeName)
					return nil
				}
				// Close the failed client
				sshManager.CloseClient(nodeName)
			}
		}

		log.Debugf("SSH not ready on node %s, retrying in 3 seconds...", nodeName)
		time.Sleep(3 * time.Second)
	}

	return fmt.Errorf("SSH service not ready after %v timeout", timeout)
}

// Legacy method removed - use createVMsSequentialFromGroups instead

// Legacy method removed - use createVMsParallelFromGroups instead

// CreateWorkerVMs creates worker nodes

func (m *Manager) InitializeAuth() error {
	if m.Config.Parallel > 1 {
		return m.initializeAuthParallel()
	}
	return m.initializeAuthSequential()
}

func (m *Manager) initializeAuthSequential() error {
	// Initialize authentication for all nodes from node groups
	for _, group := range m.Cluster.Status.Nodes {
		for _, member := range group.Members {
			err := m.initializeAuthForNode(member.Name)
			if err != nil {
				return fmt.Errorf("failed to initialize authentication for VM %s: %w", member.Name, err)
			}
		}
	}
	return nil
}

func (m *Manager) initializeAuthParallel() error {
	var wg sync.WaitGroup
	// concurrent by m.Config.Parallel
	sem := make(chan struct{}, m.Config.Parallel)

	// Count total nodes
	totalNodes := 0
	for _, group := range m.Cluster.Status.Nodes {
		totalNodes += len(group.Members)
	}

	// Channel to collect errors
	errChan := make(chan error, totalNodes)

	// Process all nodes from all groups
	for _, group := range m.Cluster.Status.Nodes {
		for _, member := range group.Members {
			wg.Add(1)
			go func(nodeName string) {
				defer wg.Done()
				sem <- struct{}{}        // acquire semaphore
				defer func() { <-sem }() // release semaphore

				err := m.initializeAuthForNode(nodeName)
				if err != nil {
					errChan <- fmt.Errorf("failed to initialize authentication for VM %s: %w", nodeName, err)
				}
			}(member.Name)
		}
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) initializeAuthForNode(nodeName string) error {
	// Check if authentication is already initialized
	if m.Cluster.HasNodeCondition(nodeName, config.ConditionTypeAuthInitialized, config.ConditionStatusTrue) {
		log.Debugf("Authentication for node %s already initialized, skipping initialization", nodeName)
		return nil
	}

	// Set condition to pending
	m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeAuthInitialized, config.ConditionStatusFalse,
		"Initializing", "Initializing node authentication")

	// Call the provider's InitAuth method
	err := m.VMProvider.InitAuth(nodeName)
	if err != nil {
		m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeAuthInitialized, config.ConditionStatusFalse,
			"InitializationFailed", fmt.Sprintf("Failed to initialize authentication: %v", err))
		return fmt.Errorf("failed to initialize authentication for node %s: %w", nodeName, err)
	}

	// Set condition to success
	m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeAuthInitialized, config.ConditionStatusTrue,
		"Initialized", "Node authentication initialized successfully")
	m.Cluster.Save()

	log.Debugf("Authentication initialized successfully for node %s", nodeName)
	return nil
}

func (m *Manager) InitializeVMs() error {
	if m.Config.Parallel > 1 {
		return m.initializeVMsParallel()
	}
	return m.initializeVMsSequential()
}

func (m *Manager) initializeVMsSequential() error {
	// Initialize all nodes from node groups
	for _, group := range m.Cluster.Status.Nodes {
		for _, member := range group.Members {
			err := m.initializeVM(member.Name)
			if err != nil {
				return fmt.Errorf("failed to initialize VM %s: %w", member.Name, err)
			}
		}
	}
	return nil
}

func (m *Manager) initializeVMsParallel() error {
	var wg sync.WaitGroup
	// concurrent by m.Config.Parallel
	sem := make(chan struct{}, m.Config.Parallel)

	// Count total nodes
	totalNodes := m.Cluster.GetTotalRunningNodes()
	errChan := make(chan error, totalNodes)

	// Initialize all nodes from node groups
	for _, group := range m.Cluster.Status.Nodes {
		for _, member := range group.Members {
			wg.Add(1)
			go func(nodeName string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				err := m.initializeVM(nodeName)
				if err != nil {
					errChan <- fmt.Errorf("failed to initialize VM %s: %w", nodeName, err)
				}
			}(member.Name)
		}
	}

	wg.Wait()
	close(errChan)

	// Check for errors
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
	if m.Cluster.HasNodeCondition(nodeName, config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue) {
		log.Debugf("Environment for node %s already initialized, skipping initialization", nodeName)
		return nil
	}

	// Ensure node status is up to date (including architecture detection)
	if node.Arch == "" {
		log.Debugf("Architecture not detected for node %s, detecting now...", nodeName)
		err := m.SetStatusForNodeByName(nodeName)
		if err != nil {
			return fmt.Errorf("failed to detect node architecture for %s: %w", nodeName, err)
		}
		// Refresh node reference after status update
		node = m.Cluster.GetNodeByName(nodeName)
		if node == nil {
			return fmt.Errorf("node %s not found after status update", nodeName)
		}
	}

	// Set condition to pending
	m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeEnvironmentInit, config.ConditionStatusFalse,
		"Initializing", "Initializing node environment")

	// Create a temporary Node structure for the initializer
	tempNode := &config.Node{
		Metadata: config.Metadata{
			Name: nodeName,
		},
		Status: config.NodeStatus{
			NodeInternalStatus: config.NodeInternalStatus{
				IP:       node.IP,
				Hostname: node.Hostname,
				Phase:    node.Phase,
				Arch:     node.Arch, // Set the architecture
				Release:  node.Release,
				Kernel:   node.Kernel,
				OS:       node.OS,
			},
		},
	}

	// Create and run the initializer
	initializer, err := initializer.NewInitializerWithOptions(m.sshRunner, tempNode, m.InitOptions)
	if err != nil {
		m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeEnvironmentInit, config.ConditionStatusFalse,
			"InitializationFailed", fmt.Sprintf("Failed to create initializer: %v", err))
		return fmt.Errorf("failed to create initializer for node %s: %w", nodeName, err)
	}

	if err := initializer.Initialize(); err != nil {
		m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeEnvironmentInit, config.ConditionStatusFalse,
			"InitializationFailed", fmt.Sprintf("Failed to initialize environment: %v", err))
		return fmt.Errorf("failed to initialize VM %s: %w", nodeName, err)
	}

	// Set condition to success
	m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue,
		"Initialized", "Node environment initialized successfully")
	m.Cluster.Save()

	return nil
}

// InitializeMaster initializes master node
func (m *Manager) InitializeMaster() (string, error) {
	log.Debugf("Initializing Master node...")

	masterName := m.Cluster.GetMasterName()
	if masterName == "" {
		return "", fmt.Errorf("no master node found in cluster")
	}

	// Update KubeManager with the actual master node name
	m.KubeManager.SetMasterNode(masterName)

	// Check if master is already initialized
	if m.Cluster.HasNodeCondition(masterName, config.ConditionTypeKubeInitialized, config.ConditionStatusTrue) {
		log.Debugf("Master node already initialized, skipping initialization")
		return m.getJoinCommand()
	}

	// Set condition to pending
	m.Cluster.SetNodeCondition(masterName, config.ConditionTypeKubeInitialized, config.ConditionStatusFalse,
		"Initializing", "Initializing Kubernetes master node")

	// Use new config system to generate config file and initialize Master
	err := m.KubeManager.InitMaster()
	if err != nil {
		m.Cluster.SetNodeCondition(masterName, config.ConditionTypeKubeInitialized, config.ConditionStatusFalse,
			"InitializationFailed", fmt.Sprintf("Failed to initialize master: %v", err))
		return "", fmt.Errorf("failed to initialize Master node: %w", err)
	}

	// Set condition to success
	m.Cluster.SetNodeCondition(masterName, config.ConditionTypeKubeInitialized, config.ConditionStatusTrue,
		"Initialized", "Master node initialized successfully")
	m.Cluster.SetNodeCondition(masterName, config.ConditionTypeNodeReady, config.ConditionStatusTrue,
		"NodeReady", "Master node is ready")
	m.Cluster.Save()

	return m.getJoinCommand()
}

// JoinWorkerNodes joins worker nodes to the cluster
func (m *Manager) JoinWorkerNodes() error {
	joinCommand, err := m.getJoinCommand()
	if err != nil {
		return fmt.Errorf("failed to get join command: %w", err)
	}

	// Join all worker nodes from node groups (excluding master group 1)
	for _, group := range m.Cluster.Status.Nodes {
		if group.GroupID != 1 { // Skip master group
			for _, member := range group.Members {
				err := m.JoinWorkerNode(member.Name, joinCommand)
				if err != nil {
					return err
				}
			}
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
	if m.Cluster.HasNodeCondition(nodeName, config.ConditionTypeJoinedCluster, config.ConditionStatusTrue) {
		log.Debugf("Node %s already joined the cluster, skipping join", nodeName)
		return nil
	}

	// Set condition to pending
	m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeJoinedCluster, config.ConditionStatusFalse,
		"Joining", "Joining node to the cluster")

	// Join the node to the cluster
	err := m.KubeManager.JoinNode(nodeName, joinCommand)
	if err != nil {
		m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeJoinedCluster, config.ConditionStatusFalse,
			"JoinFailed", fmt.Sprintf("Failed to join node to cluster: %v", err))
		return err
	}

	// Set condition to success
	m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeJoinedCluster, config.ConditionStatusTrue,
		"Joined", "Node joined the cluster successfully")
	m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeNodeReady, config.ConditionStatusTrue,
		"NodeReady", "Worker node is ready")

	return m.Cluster.Save()
}

func (m *Manager) AddWorkerNodes(cpu int, memory int, disk int, count int) error {
	// Check if cluster is healthy before adding nodes
	healthy, reason := m.IsClusterHealthy()
	if !healthy {
		return fmt.Errorf("cluster is not in a healthy state for adding nodes: %s. Please run 'ohmykube up' to fix cluster state first", reason)
	}

	// Add new nodes as requested
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

	// Create resource requirements
	resources := config.ResourceRequests{
		CPU:     fmt.Sprintf("%d", cpu),
		Memory:  fmt.Sprintf("%dGi", memory),
		Storage: fmt.Sprintf("%dGi", disk),
	}

	// Get template from config, fallback to default if not set
	template := m.Config.Template
	if template == "" {
		template = "ubuntu-24.04" // Default template
	}

	// Find matching group or create new one
	groupID := m.Cluster.AddNodeToWorkerGroup(template, resources)

	log.Infof("Adding worker node to group %d", groupID)

	node, err := m.setupVM("", config.RoleWorker, groupID, cpu, memory, disk)
	if err != nil {
		return err
	}
	nodeName := node.Name
	m.initializeAuthForNode(nodeName)
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
	m.Cluster.SetNodeCondition(nodeName, config.ConditionTypeJoinedCluster, config.ConditionStatusFalse,
		"Joining", "Joining node to the cluster")

	return m.JoinWorkerNode(nodeName, joinCmd)
}

// findIncompleteWorkerNodes finds worker nodes that exist but are not fully set up
func (m *Manager) findIncompleteWorkerNodes() []string {
	var incompleteNodes []string

	// Check all worker node groups (excluding master group 1)
	for _, group := range m.Cluster.Status.Nodes {
		if group.GroupID == 1 { // Skip master group
			continue
		}

		for _, member := range group.Members {
			// Check if node exists but is not fully joined to the cluster
			if !m.Cluster.HasNodeCondition(member.Name, config.ConditionTypeJoinedCluster, config.ConditionStatusTrue) {
				// Verify the VM actually exists
				status, err := m.VMProvider.Status(member.Name)
				if err == nil && status.IsRunning() {
					incompleteNodes = append(incompleteNodes, member.Name)
				}
			}
		}
	}

	return incompleteNodes
}

// resumeWorkerNodeSetup resumes the setup process for an incomplete worker node
func (m *Manager) resumeWorkerNodeSetup(nodeName string) error {
	var err error
	// Check if authentication initialization is needed
	if !m.Cluster.HasNodeCondition(nodeName, config.ConditionTypeAuthInitialized, config.ConditionStatusTrue) {
		log.Infof("Initializing authentication for node %s", nodeName)
		err = m.initializeAuthForNode(nodeName)
		if err != nil {
			return fmt.Errorf("failed to initialize authentication for node %s: %w", nodeName, err)
		}
	}
	// Ensure node status is up to date
	err = m.SetStatusForNodeByName(nodeName)
	if err != nil {
		return fmt.Errorf("failed to update status for node %s: %w", nodeName, err)
	}

	// Check if environment initialization is needed
	if !m.Cluster.HasNodeCondition(nodeName, config.ConditionTypeEnvironmentInit, config.ConditionStatusTrue) {
		log.Infof("Initializing environment for node %s", nodeName)
		err = m.initializeVM(nodeName)
		if err != nil {
			return fmt.Errorf("failed to initialize environment for node %s: %w", nodeName, err)
		}
	}

	// Check if node needs to join the cluster
	if !m.Cluster.HasNodeCondition(nodeName, config.ConditionTypeJoinedCluster, config.ConditionStatusTrue) {
		log.Infof("Joining node %s to cluster", nodeName)

		// Get join command
		joinCmd, err := m.getJoinCommand()
		if err != nil {
			return fmt.Errorf("failed to get join command for node %s: %w", nodeName, err)
		}

		// Join the node to the cluster
		err = m.JoinWorkerNode(nodeName, joinCmd)
		if err != nil {
			return fmt.Errorf("failed to join node %s to cluster: %w", nodeName, err)
		}
	}

	log.Infof("Successfully resumed setup for worker node: %s", nodeName)
	return nil
}

// DeleteNode deletes a node from the cluster
func (m *Manager) DeleteNode(nodeName string, force bool) error {
	log.Infof("Deleting node %s...", nodeName)
	// Check if node exists
	nodeInfo := m.Cluster.GetNodeByName(nodeName)
	if nodeInfo == nil {
		return fmt.Errorf("node %s does not exist", nodeName)
	}

	// Check if it's a master node and there are worker nodes
	isMaster := false
	for _, group := range m.Cluster.Status.Nodes {
		if group.GroupID == 1 {
			for _, member := range group.Members {
				if member.Name == nodeName {
					isMaster = true
					break
				}
			}
		}
	}

	if isMaster && m.Cluster.GetTotalRunningNodes() > 1 {
		return fmt.Errorf("cannot delete Master node, please delete the entire cluster first")
	}

	// Evict node from Kubernetes
	if !force && nodeInfo.Phase == config.PhaseRunning {
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
	if !force && nodeInfo.Phase == config.PhaseRunning {
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

// IsClusterHealthy checks if the cluster is in a healthy state for adding nodes
func (m *Manager) IsClusterHealthy() (bool, string) {
	// Check if cluster is ready
	if !m.Cluster.HasCondition(config.ConditionTypeClusterReady, config.ConditionStatusTrue) {
		return false, "cluster is not ready"
	}

	// Check if all nodes are ready
	if !m.Cluster.HasAllNodeCondition(config.ConditionTypeNodeReady, config.ConditionStatusTrue) {
		return false, "not all nodes are ready"
	}

	// Check for incomplete worker nodes
	incompleteNodes := m.findIncompleteWorkerNodes()
	if len(incompleteNodes) > 0 {
		return false, fmt.Sprintf("found %d incomplete worker nodes: %v", len(incompleteNodes), incompleteNodes)
	}

	return true, ""
}

// canResumeClusterCreation checks if there are any conditions set that indicate
// a previous cluster creation attempt that can be resumed
func (m *Manager) canResumeClusterCreation() bool {
	// If cluster is already fully ready, we should not resume cluster creation
	// This prevents 'ohmykube up' from recreating an already completed cluster
	if m.Cluster.HasCondition(config.ConditionTypeClusterReady, config.ConditionStatusTrue) {
		return false
	}

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
	for _, group := range m.Cluster.Status.Nodes {
		for _, member := range group.Members {
			for _, cond := range member.Conditions {
				if (cond.Type == config.ConditionTypeVMCreated ||
					cond.Type == config.ConditionTypeEnvironmentInit ||
					cond.Type == config.ConditionTypeKubeInitialized ||
					cond.Type == config.ConditionTypeJoinedCluster) &&
					cond.Status == config.ConditionStatusTrue {
					return true
				}
			}
		}
	}

	return false
}

// executeParallelSteps executes worker join, CNI, CSI, and LB installation in parallel
func (m *Manager) executeParallelSteps(progress *log.MultiStepProgress, baseStepIndex int) error {
	// Define the parallel steps
	type parallelStep struct {
		name        string
		description string
		condition   config.ConditionType
		skipCheck   func() bool
		execute     func() error
		skipReason  string
	}

	steps := []parallelStep{
		{
			name:        "worker-join",
			description: "Joining worker nodes",
			condition:   config.ConditionTypeWorkersJoined,
			skipCheck: func() bool {
				return m.Cluster.HasCondition(config.ConditionTypeWorkersJoined, config.ConditionStatusTrue) &&
					m.Cluster.HasAllNodeCondition(config.ConditionTypeJoinedCluster, config.ConditionStatusTrue)
			},
			execute: func() error {
				log.Debugf("Joining Worker nodes to the cluster...")
				err := m.JoinWorkerNodes()
				if err != nil {
					m.Cluster.SetCondition(config.ConditionTypeWorkersJoined, config.ConditionStatusFalse,
						"WorkersJoinFailed", fmt.Sprintf("Failed to join workers: %v", err))
					m.Cluster.Save()
					return fmt.Errorf("failed to join Worker nodes to the cluster: %w", err)
				}

				m.Cluster.SetCondition(config.ConditionTypeWorkersJoined, config.ConditionStatusTrue,
					"WorkersJoined", "All worker nodes joined the cluster successfully")
				m.Cluster.Save()
				return nil
			},
			skipReason: "worker joining already completed",
		},
		{
			name:        "cni-install",
			description: "Installing CNI",
			condition:   config.ConditionTypeCNIInstalled,
			skipCheck: func() bool {
				return m.Config.CNI == "none" || m.Cluster.HasCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusTrue)
			},
			execute: func() error {
				log.Debugf("Installing %s CNI...", m.Config.CNI)
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
				return nil
			},
			skipReason: func() string {
				if m.Config.CNI == "none" {
					return "CNI installation skipped as configured"
				}
				return "CNI installation already completed"
			}(),
		},
		{
			name:        "csi-install",
			description: "Installing CSI",
			condition:   config.ConditionTypeCSIInstalled,
			skipCheck: func() bool {
				return m.Config.CSI == "none" || m.Cluster.HasCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusTrue)
			},
			execute: func() error {
				log.Debugf("Installing %s CSI...", m.Config.CSI)
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
				return nil
			},
			skipReason: func() string {
				if m.Config.CSI == "none" {
					return "CSI installation skipped as configured"
				}
				return "CSI installation already completed"
			}(),
		},
		{
			name:        "lb-install",
			description: "Installing LoadBalancer",
			condition:   config.ConditionTypeLBInstalled,
			skipCheck: func() bool {
				return !(m.Config.LB == "metallb" && m.InitOptions.EnableIPVS()) ||
					m.Cluster.HasCondition(config.ConditionTypeLBInstalled, config.ConditionStatusTrue)
			},
			execute: func() error {
				log.Debugf("Installing MetalLB LoadBalancer...")
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
				return nil
			},
			skipReason: func() string {
				if !(m.Config.LB == "metallb" && m.InitOptions.EnableIPVS()) {
					return "LoadBalancer installation not required"
				}
				return "LoadBalancer installation already completed"
			}(),
		},
	}

	// Filter out steps that should be skipped
	var activeSteps []parallelStep
	for _, step := range steps {
		if !step.skipCheck() {
			activeSteps = append(activeSteps, step)
		} else {
			log.Debugf("Skipping %s: %s", step.description, step.skipReason)
		}
	}

	// If no steps need to be executed, return early
	if len(activeSteps) == 0 {
		log.Debugf("All parallel steps already completed, skipping parallel execution")
		return nil
	}

	// Execute steps in parallel
	var wg sync.WaitGroup
	errors := make([]error, len(activeSteps))
	stepIndexMap := make(map[string]int)

	// Start all active steps
	for i, step := range activeSteps {
		stepIndex := baseStepIndex + i
		stepIndexMap[step.name] = stepIndex

		// Start the step in progress
		progress.StartStep(stepIndex)

		wg.Add(1)
		go func(stepIdx int, s parallelStep) {
			defer wg.Done()

			// Execute the step
			err := s.execute()
			errors[stepIdx] = err

			// Update progress based on result
			if err != nil {
				progress.FailStep(stepIndexMap[s.name], err)
			} else {
				progress.CompleteStep(stepIndexMap[s.name])
			}
		}(i, step)
	}

	// Wait for all steps to complete
	wg.Wait()

	// Check for any errors
	for i, err := range errors {
		if err != nil {
			return fmt.Errorf("parallel step %s failed: %w", activeSteps[i].name, err)
		}
	}

	log.Debugf("All parallel steps completed successfully")
	return nil
}
