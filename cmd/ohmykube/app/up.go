package app

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/controller"
	"github.com/monshunter/ohmykube/pkg/initializer"
	"github.com/monshunter/ohmykube/pkg/log"
	myProvider "github.com/monshunter/ohmykube/pkg/provider"
	"github.com/monshunter/ohmykube/pkg/ssh"
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
		var cls *config.Cluster
		var err error
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
				proxyMode = cls.Spec.ProxyMode
				k8sVersion = cls.Spec.K8sVersion
				if cls.Spec.LB != "" {
					lb = cls.Spec.LB
				} else if lb != "" {
					cls.Spec.LB = lb
				}
				log.Debugf("lb: %s, cls.Spec.LB: %s", lb, cls.Spec.LB)
				log.Debugf("Using existing valid cluster configuration")
			}
		}

		sshConfig, err := ssh.NewSSHConfig(password, sshKeyFile, sshPubKeyFile)
		if err != nil {
			return err
		}

		providerType := myProvider.ProviderType(provider)
		if !providerType.IsValid() {
			return fmt.Errorf("invalid provider type: %s, currently only 'lima' is supported", providerType)
		}
		// Create environment initialization options
		// Create cluster configuration
		cfg := config.NewConfig(
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

// isValidClusterConfig validates that a cluster configuration is complete and valid
func isValidClusterConfig(cls *config.Cluster) bool {
	if cls == nil {
		return false
	}

	// Check essential fields that must not be empty
	if cls.Spec.K8sVersion == "" {
		log.Debugf("Invalid cluster config: K8sVersion is empty")
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
	upCmd.Flags().StringVar(&cni, "cni", "flannel", "CNI type to install (flannel, cilium, none)")
	upCmd.Flags().StringVar(&csi, "csi", "local-path-provisioner",
		"CSI type to install (local-path-provisioner, rook-ceph, none)")
	upCmd.Flags().StringVar(&lb, "lb", "",
		`LoadBalancer (only "metallb" is supported for now, and ipvs mode is required), leave empty to disable`)
}
