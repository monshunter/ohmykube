package app

import (
	"encoding/json"
	"fmt"
	"strings"

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
	configFile        string
	// Node metadata options
	masterLabels      []string
	workerLabels      []string
	masterAnnotations []string
	workerAnnotations []string
	masterTaints      []string
	workerTaints      []string

	// Custom initialization options for master nodes
	masterHookPreSystemInit  []string
	masterHookPostSystemInit []string
	masterHookPreK8sInit     []string
	masterHookPostK8sInit    []string
	masterUploadFiles        []string
	masterUploadDirs         []string

	// Custom initialization options for worker nodes
	workerHookPreSystemInit  []string
	workerHookPostSystemInit []string
	workerHookPreK8sInit     []string
	workerHookPostK8sInit    []string
	workerUploadFiles        []string
	workerUploadDirs         []string

	// Addon configuration
	addonsFlag []string // --addon (repeatable)
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Create a k8s cluster (with CNI, CSI, MetalLB)",
	Long: `Create a VM-based k8s cluster with the following components:
- Optional CNI: flannel(default) or cilium
- Optional CSI: local-path-provisioner(default) or rook-ceph
- MetalLB as LoadBalancer implementation`,
	Args: cobra.NoArgs,
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

		// Process command line addons and add them to the cluster spec
		addonSpecs, err := processAddonFlags()
		if err != nil {
			log.Errorf("❌ Invalid addon configuration: %v", err)
			return fmt.Errorf("invalid addon configuration: %w", err)
		}

		var cls *config.Cluster

		// Load cluster from config file if specified
		if configFile != "" {
			log.Infof("📄 Loading cluster configuration from file: %s", configFile)
			loadedCluster, err := config.LoadClusterFromFile(configFile)
			if err != nil {
				return fmt.Errorf("failed to load cluster from config file: %w", err)
			}
			cls = loadedCluster
			// Set cluster name from loaded configuration
			clusterName = cls.Name
			log.Infof("🏷️  Using cluster name from config file: %s", clusterName)
		}

		if config.CheckExists(clusterName) {
			cls, err = config.LoadCluster(clusterName)
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
			cfg.SetUpdateSystem(cls.GetUpdateSystem())

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

			// Parse and set node metadata
			masterMetadata, err := parseNodeMetadata(masterLabels, masterAnnotations, masterTaints)
			if err != nil {
				return fmt.Errorf("failed to parse master node metadata: %w", err)
			}
			cfg.SetMasterMetadata(masterMetadata)

			workerMetadata, err := parseNodeMetadata(workerLabels, workerAnnotations, workerTaints)
			if err != nil {
				return fmt.Errorf("failed to parse worker node metadata: %w", err)
			}
			cfg.SetWorkerMetadata(workerMetadata)

			// Parse and set custom initialization configuration
			masterCustomInit, err := parseCustomInitConfig(masterHookPreSystemInit, masterHookPostSystemInit, masterHookPreK8sInit, masterHookPostK8sInit, masterUploadFiles, masterUploadDirs)
			if err != nil {
				return fmt.Errorf("failed to parse master custom initialization config: %w", err)
			}
			cfg.SetMasterCustomInit(masterCustomInit)

			workerCustomInit, err := parseCustomInitConfig(workerHookPreSystemInit, workerHookPostSystemInit, workerHookPreK8sInit, workerHookPostK8sInit, workerUploadFiles, workerUploadDirs)
			if err != nil {
				return fmt.Errorf("failed to parse worker custom initialization config: %w", err)
			}
			cfg.SetWorkerCustomInit(workerCustomInit)

			// Create new cluster
			cls = config.NewCluster(cfg)
			log.Infof("🆕 Creating new cluster")
		}

		// Get default initialization options and modify required fields
		initOptions := initializer.DefaultInitOptions()
		initOptions.DisableSwap = !enableSwap // If enableSwap is true, DisableSwap is false
		initOptions.ProxyMode = initializer.ProxyMode(proxyMode)
		initOptions.K8SVersion = k8sVersion
		if cls != nil {
			initOptions.UpdateSystem = cls.GetUpdateSystem()
			// Add command line addons to cluster spec
			if len(addonSpecs) > 0 {
				cls.AddCommandLineAddons(addonSpecs)
				log.Infof("🔌 Merged %d command line addon(s) into cluster spec", len(addonSpecs))
			}

			// Validate all addon specifications - terminate on errors
			if err := cls.ValidateAllAddons(); err != nil {
				return fmt.Errorf("addon validation failed: %w", err)
			}
		} else {
			initOptions.UpdateSystem = updateSystem
			// For new clusters, validate addon specifications if provided
			if len(addonSpecs) > 0 {
				// Create temporary cluster to validate addons
				tempCluster := &config.Cluster{Spec: config.ClusterSpec{Addons: addonSpecs}}
				if err := tempCluster.ValidateAllAddons(); err != nil {
					return fmt.Errorf("addon validation failed: %w", err)
				}
			}
		}
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

		// Set current cluster context after successful creation
		if err := config.SetCurrentCluster(clusterName); err != nil {
			log.Warningf("Failed to set current cluster context: %v", err)
			// Don't return error as cluster creation was successful
		} else {
			log.Infof("✅ Set current cluster context to: %s", clusterName)
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

// parseLabelsAnnotations parses key=value format strings into a map
func parseLabelsAnnotations(items []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, item := range items {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid format '%s', expected key=value", item)
		}
		result[parts[0]] = parts[1]
	}
	return result, nil
}

// parseTaints parses key=value:effect format strings into NodeTaint slice
func parseTaints(items []string) ([]config.Taint, error) {
	var taints []config.Taint
	for _, item := range items {
		// Format: key=value:effect or key:effect (value is optional)
		parts := strings.Split(item, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid taint format '%s', expected key=value:effect or key:effect", item)
		}

		keyValue := parts[0]
		effect := parts[1]

		// Validate effect
		if effect != "NoSchedule" && effect != "NoExecute" && effect != "PreferNoSchedule" {
			return nil, fmt.Errorf("invalid taint effect '%s', must be NoSchedule, NoExecute, or PreferNoSchedule", effect)
		}

		// Parse key and value
		var key, value string
		if strings.Contains(keyValue, "=") {
			kvParts := strings.SplitN(keyValue, "=", 2)
			key = kvParts[0]
			value = kvParts[1]
		} else {
			key = keyValue
			value = ""
		}

		taints = append(taints, config.Taint{
			Key:    key,
			Value:  value,
			Effect: effect,
		})
	}
	return taints, nil
}

// parseNodeMetadata parses command line flags into NodeMetadata
func parseNodeMetadata(labels, annotations, taints []string) (config.NodeMetadata, error) {
	metadata := config.NodeMetadata{}

	// Parse labels
	if len(labels) > 0 {
		parsedLabels, err := parseLabelsAnnotations(labels)
		if err != nil {
			return metadata, fmt.Errorf("failed to parse labels: %w", err)
		}
		metadata.Labels = parsedLabels
	}

	// Parse annotations
	if len(annotations) > 0 {
		parsedAnnotations, err := parseLabelsAnnotations(annotations)
		if err != nil {
			return metadata, fmt.Errorf("failed to parse annotations: %w", err)
		}
		metadata.Annotations = parsedAnnotations
	}

	// Parse taints
	if len(taints) > 0 {
		parsedTaints, err := parseTaints(taints)
		if err != nil {
			return metadata, fmt.Errorf("failed to parse taints: %w", err)
		}
		metadata.Taints = parsedTaints
	}

	return metadata, nil
}

// processAddonFlags processes the --addon command line flags
func processAddonFlags() ([]config.AddonSpec, error) {
	var specs []config.AddonSpec

	for _, addonJSON := range addonsFlag {
		var addon config.AddonSpec
		if err := json.Unmarshal([]byte(addonJSON), &addon); err != nil {
			return nil, fmt.Errorf("invalid addon JSON: %s, error: %w", addonJSON, err)
		}

		// Validate addon spec (using minimal validation for command line)
		if err := addon.ValidateMinimal(); err != nil {
			return nil, fmt.Errorf("invalid addon spec: %w", err)
		}

		specs = append(specs, addon)
	}

	return specs, nil
}

func init() {
	// Add command line parameters
	upCmd.Flags().StringVarP(&configFile, "file", "f", "", "Path to cluster configuration file")
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

	// Node metadata flags
	upCmd.Flags().StringArrayVar(&masterLabels, "master-labels", []string{},
		`Labels for master nodes (format: key=value). Can be specified multiple times`)
	upCmd.Flags().StringArrayVar(&workerLabels, "worker-labels", []string{},
		`Labels for worker nodes (format: key=value). Can be specified multiple times`)
	upCmd.Flags().StringArrayVar(&masterAnnotations, "master-annotations", []string{},
		`Annotations for master nodes (format: key=value). Can be specified multiple times`)
	upCmd.Flags().StringArrayVar(&workerAnnotations, "worker-annotations", []string{},
		`Annotations for worker nodes (format: key=value). Can be specified multiple times`)
	upCmd.Flags().StringArrayVar(&masterTaints, "master-taints", []string{},
		`Taints for master nodes (format: key=value:effect). Can be specified multiple times. Effect can be NoSchedule|NoExecute|PreferNoSchedule`)
	upCmd.Flags().StringArrayVar(&workerTaints, "worker-taints", []string{},
		`Taints for worker nodes (format: key=value:effect). Can be specified multiple times. Effect can be NoSchedule|NoExecute|PreferNoSchedule`)

	// Master custom initialization flags
	upCmd.Flags().StringArrayVar(&masterHookPreSystemInit, "master-hook-pre-system-init", []string{},
		`Hook scripts to run before system update on master nodes (format: /path/to/script.sh). Multiple files supported with comma separation`)
	upCmd.Flags().StringArrayVar(&masterHookPostSystemInit, "master-hook-post-system-init", []string{},
		`Hook scripts to run after system update, before K8s install on master nodes (format: /path/to/script.sh). Multiple files supported with comma separation`)
	upCmd.Flags().StringArrayVar(&masterHookPreK8sInit, "master-hook-pre-k8s-init", []string{},
		`Hook scripts to run before K8s components install on master nodes (format: /path/to/script.sh). Multiple files supported with comma separation`)
	upCmd.Flags().StringArrayVar(&masterHookPostK8sInit, "master-hook-post-k8s-init", []string{},
		`Hook scripts to run after all initialization on master nodes (format: /path/to/script.sh). Multiple files supported with comma separation`)
	upCmd.Flags().StringArrayVar(&masterUploadFiles, "master-upload-file", []string{},
		`Files to upload to master nodes (format: local_path:remote_path[:mode[:owner]]). Example: /tmp/config.yml:/etc/config.yml:0644:root:root`)
	upCmd.Flags().StringArrayVar(&masterUploadDirs, "master-upload-dir", []string{},
		`Directories to upload to master nodes (format: local_dir:remote_dir[:mode[:owner]]). Recursively copies entire directory`)

	// Worker custom initialization flags
	upCmd.Flags().StringArrayVar(&workerHookPreSystemInit, "worker-hook-pre-system-init", []string{},
		`Hook scripts to run before system update on worker nodes (format: /path/to/script.sh). Multiple files supported with comma separation`)
	upCmd.Flags().StringArrayVar(&workerHookPostSystemInit, "worker-hook-post-system-init", []string{},
		`Hook scripts to run after system update, before K8s install on worker nodes (format: /path/to/script.sh). Multiple files supported with comma separation`)
	upCmd.Flags().StringArrayVar(&workerHookPreK8sInit, "worker-hook-pre-k8s-init", []string{},
		`Hook scripts to run before K8s components install on worker nodes (format: /path/to/script.sh). Multiple files supported with comma separation`)
	upCmd.Flags().StringArrayVar(&workerHookPostK8sInit, "worker-hook-post-k8s-init", []string{},
		`Hook scripts to run after all initialization on worker nodes (format: /path/to/script.sh). Multiple files supported with comma separation`)
	upCmd.Flags().StringArrayVar(&workerUploadFiles, "worker-upload-file", []string{},
		`Files to upload to worker nodes (format: local_path:remote_path[:mode[:owner]]). Example: /tmp/config.yml:/etc/config.yml:0644:root:root`)
	upCmd.Flags().StringArrayVar(&workerUploadDirs, "worker-upload-dir", []string{},
		`Directories to upload to worker nodes (format: local_dir:remote_dir[:mode[:owner]]). Recursively copies entire directory`)

	// Addon parameter
	upCmd.Flags().StringSliceVar(&addonsFlag, "addon", nil,
		`Add addon from JSON spec (repeatable). Format: '{"name":"app-name","type":"helm|manifest","repo":"...","chart":"...","version":"..."}' for helm or '{"name":"app-name","type":"manifest","url":"...","version":"..."}' for manifest`)
}
