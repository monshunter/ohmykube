package app

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/controller"
	"github.com/monshunter/ohmykube/pkg/initializer"
	"github.com/monshunter/ohmykube/pkg/log"
	myProvider "github.com/monshunter/ohmykube/pkg/provider"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/monshunter/ohmykube/pkg/utils"
	"github.com/spf13/cobra"
)

var (
	// Cluster configuration options
	k8sVersion        string
	workersCount      int
	masterMemory      int
	masterCPU         int
	workerMemory      int
	workerCPU         int
	masterDisk        int
	workerDisk        int
	enableSwap        bool
	kubeadmConfigPath string
	proxyMode         string
	cni               string
	csi               string
	lb                string
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Create a k8s cluster (with CNI, CSI, MetalLB)",
	Long: `Create a VM-based k8s cluster with the following components:
- Optional CNI: flannel(default) or cilium
- Optional CSI: local-path-provisioner(default) or rook-ceph
- MetalLB as LoadBalancer implementation`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Set up graceful shutdown handling
		shutdownHandler := NewGracefulShutdownHandler()
		defer shutdownHandler.Close()

		// Normalize and validate k8sVersion first
		normalizedVersion, err := utils.NormalizeK8sVersion(k8sVersion)
		if err != nil {
			log.Errorf("Invalid Kubernetes version '%s': %v", k8sVersion, err)
			return fmt.Errorf("invalid Kubernetes version '%s': %w", k8sVersion, err)
		}
		k8sVersion = normalizedVersion
		log.Debugf("Normalized Kubernetes version: %s", k8sVersion)

		// Auto-configure proxy-mode for MetalLB
		if lb == "metallb" && proxyMode != "ipvs" {
			log.Infof("MetalLB requires IPVS mode, automatically setting proxy-mode to 'ipvs'")
			proxyMode = "ipvs"
		}

		var cls *config.Cluster
		if config.CheckExists(clusterName) {
			cls, err = config.Load(clusterName)
			if err != nil {
				log.Errorf("Failed to load cluster information: %v", err)
				return fmt.Errorf("failed to load cluster information: %w", err)
			}

			// Validate the loaded cluster configuration
			if !isValidClusterConfig(cls) {
				log.Warningf("Found invalid cluster configuration for '%s', removing corrupted configuration...", clusterName)
				if removeErr := config.RemoveCluster(clusterName); removeErr != nil {
					log.Errorf("Failed to remove corrupted cluster configuration: %v", removeErr)
				} else {
					log.Infof("Corrupted cluster configuration removed. Starting fresh cluster creation.")
				}
				cls = nil // Reset to create a new cluster
			} else {
				// Use valid stored configuration
				provider = cls.Spec.Provider

				if cls.GetProxyMode() != proxyMode {
					log.Warningf(`Ignore changing the proxy mode from "%s" to "%s" because the proxy-mode of a running cluster cannot be modified.`,
						cls.GetProxyMode(), proxyMode)
				}

				proxyMode = cls.GetProxyMode()
				// Normalize the stored k8sVersion as well
				storedVersion, versionErr := utils.NormalizeK8sVersion(cls.GetKubernetesVersion())
				if versionErr != nil {
					return fmt.Errorf("invalid stored Kubernetes version '%s': %w", cls.GetKubernetesVersion(), versionErr)
				} else {
					k8sVersion = storedVersion
					log.Debugf("Using stored Kubernetes version: %s", k8sVersion)
				}
				if cls.GetLoadBalancer() != "" {
					lb = cls.GetLoadBalancer()
				} else if lb != "" {
					cls.Spec.Networking.LoadBalancer = lb
				}

				// validation of LoadBalancer and proxy-mode compatibility
				if err := validateLBProxyModeCompatibility(lb, proxyMode); err != nil {
					return err
				}

				log.Debugf("Using existing valid cluster configuration")
			}
		}

		sshConfig, err := ssh.NewSSHConfig(password, clusterName)
		if err != nil {
			return err
		}

		providerType := myProvider.ProviderType(provider)
		if !providerType.IsValid() {
			return fmt.Errorf("invalid provider type: %s, currently only 'lima' is supported", providerType)
		}
		// Create cluster configuration
		var cfg *config.Config

		if cls != nil {
			// Use existing cluster configuration for resume
			// Get template from existing node groups (use master template as default)
			existingTemplate := template // Use command line template as fallback
			if len(cls.Spec.Nodes.Master) > 0 {
				existingTemplate = cls.Spec.Nodes.Master[0].Template
			} else if len(cls.Spec.Nodes.Workers) > 0 {
				existingTemplate = cls.Spec.Nodes.Workers[0].Template
			}

			cfg = &config.Config{
				Name:     cls.Name,
				Provider: cls.Spec.Provider,
				Template: existingTemplate,
				Parallel: parallel, // Allow override from command line
				Master: config.Resource{
					CPU:    masterCPU,    // Allow override from command line
					Memory: masterMemory, // Allow override from command line
					Disk:   masterDisk,   // Allow override from command line
				},
				Workers: []config.Resource{
					{
						CPU:    workerCPU,    // Allow override from command line
						Memory: workerMemory, // Allow override from command line
						Disk:   workerDisk,   // Allow override from command line
					},
				},
			}
			cfg.SetKubernetesVersion(cls.GetKubernetesVersion())
			cfg.SetCNIType(cls.GetCNI())
			cfg.SetCSIType(cls.GetCSI())
			cfg.SetLBType(cls.GetLoadBalancer())
			cfg.SetUpdateSystem(updateSystem)

			log.Infof("🔄 Resuming cluster creation from previous state")
		} else {
			// Create new cluster configuration
			cfg = config.NewConfig(
				clusterName,
				workersCount,
				proxyMode,
				config.Resource{
					CPU:    masterCPU,
					Memory: masterMemory,
					Disk:   masterDisk,
				}, config.Resource{
					CPU:    workerCPU,
					Memory: workerMemory,
					Disk:   workerDisk,
				})
			cfg.SetKubernetesVersion(k8sVersion)
			cfg.SetProvider(providerType.String())
			cfg.SetTemplate(template)
			cfg.SetCNIType(cni)
			cfg.SetUpdateSystem(updateSystem)
			cfg.SetLBType(lb)
			cfg.SetCSIType(csi)
			cfg.SetParallel(parallel)

			// Create new cluster
			cls = config.NewCluster(cfg)
			log.Infof("🆕 Creating new cluster")
		}

		// Get default initialization options and modify required fields
		initOptions := initializer.DefaultInitOptions()
		initOptions.DisableSwap = !enableSwap // If enableSwap is true, DisableSwap is false
		initOptions.ProxyMode = initializer.ProxyMode(proxyMode)
		initOptions.K8SVersion = k8sVersion
		initOptions.UpdateSystem = updateSystem

		// Create cluster manager and pass options
		manager, err := controller.NewManager(cfg, sshConfig, cls, nil)
		if err != nil {
			return err
		}

		// Set manager for graceful shutdown
		shutdownHandler.SetManager(manager)

		// Set environment initialization options
		manager.SetInitOptions(initOptions)

		// Set custom kubeadm config (if provided)
		if kubeadmConfigPath != "" {
			manager.SetKubeadmConfigPath(kubeadmConfigPath)
		}

		// Create cluster
		defer manager.Close()
		err = manager.CreateCluster()
		if err != nil {
			log.Errorf(`A failure has occurred.
			 You can either re-execute "ohmykube up" after the problem is fixed or directly. 
			 The process will resume from where it failed.`)
			return err
		}

		return nil
	},
}

// validateLBProxyModeCompatibility validates that LoadBalancer and proxy-mode are compatible
func validateLBProxyModeCompatibility(lb, proxyMode string) error {
	if lb == "metallb" && proxyMode != "ipvs" {
		return fmt.Errorf("MetalLB requires IPVS proxy mode, but proxy-mode is set to '%s'. MetalLB will not work properly with iptables mode", proxyMode)
	}
	return nil
}

// isValidClusterConfig validates that a cluster configuration is complete and valid
func isValidClusterConfig(cls *config.Cluster) bool {
	if cls == nil {
		return false
	}

	// Check essential fields that must not be empty
	if cls.GetKubernetesVersion() == "" {
		log.Debugf("Invalid cluster config: KubernetesVersion is empty")
		return false
	}

	if cls.Spec.Provider == "" {
		log.Debugf("Invalid cluster config: Provider is empty")
		return false
	}

	if cls.Name == "" {
		log.Debugf("Invalid cluster config: Name is empty")
		return false
	}

	// Check if provider type is valid
	providerType := myProvider.ProviderType(cls.Spec.Provider)
	if !providerType.IsValid() {
		log.Debugf("Invalid cluster config: Provider type is invalid: %s", cls.Spec.Provider)
		return false
	}

	// Validate LoadBalancer and proxy-mode compatibility
	if err := validateLBProxyModeCompatibility(cls.GetLoadBalancer(), cls.GetProxyMode()); err != nil {
		log.Debugf("Invalid cluster config: %v", err)
		return false
	}

	log.Debugf("Cluster configuration is valid")
	return true
}

func init() {
	// Add command line parameters
	upCmd.Flags().IntVarP(&workersCount, "workers", "w", 2, "Number of worker nodes")
	upCmd.Flags().IntVar(&masterMemory, "master-memory", 4, "Master node memory (GB)")
	upCmd.Flags().IntVar(&masterCPU, "master-cpu", 2, "Master node CPU cores")
	upCmd.Flags().IntVar(&workerMemory, "worker-memory", 2, "Worker node memory (GB)")
	upCmd.Flags().IntVar(&workerCPU, "worker-cpu", 1, "Worker node CPU cores")
	upCmd.Flags().IntVar(&masterDisk, "master-disk", 20, "Master node disk size (GB)")
	upCmd.Flags().IntVar(&workerDisk, "worker-disk", 10, "Worker node disk size (GB)")
	upCmd.Flags().StringVar(&k8sVersion, "k8s-version", "v1.33.0", "Kubernetes version")
	upCmd.Flags().BoolVar(&enableSwap, "enable-swap", false, "Enable Swap (only for K8s 1.28+)")
	upCmd.Flags().StringVar(&kubeadmConfigPath, "kubeadm-config", "", "Custom kubeadm config file path")
	upCmd.Flags().StringVar(&proxyMode, "proxy-mode", "iptables", "Proxy mode (iptables or ipvs), can only be set once")
	upCmd.Flags().StringVar(&cni, "cni", "flannel", "CNI type to install (flannel, cilium, none)")
	upCmd.Flags().StringVar(&csi, "csi", "local-path-provisioner",
		"CSI type to install (local-path-provisioner, rook-ceph, none)")
	upCmd.Flags().StringVar(&lb, "lb", "",
		`LoadBalancer (only "metallb" is supported for now, automatically sets proxy-mode to ipvs), leave empty to disable`)
}
