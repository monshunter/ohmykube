package cmd

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/initializer"
	myLauncher "github.com/monshunter/ohmykube/pkg/launcher"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/manager"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	// Cluster configuration options
	k8sVersion         string
	workersCount       int
	masterMemory       int
	masterCPU          int
	workerMemory       int
	workerCPU          int
	masterDisk         int
	workerDisk         int
	enableSwap         bool
	kubeadmConfigPath  string
	cniType            string
	csiType            string
	downloadKubeconfig bool
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Create a k8s cluster (with CNI, CSI, MetalLB)",
	Long: `Create a VM-based k8s cluster with the following components:
- Optional CNI: flannel(default) or cilium
- Optional CSI: local-path-provisioner(default) or rook-ceph
- MetalLB as LoadBalancer implementation`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var cls *cluster.Cluster
		var err error
		if cluster.CheckExists(clusterName) {
			cls, err = cluster.Load(clusterName)
			if err != nil {
				log.Errorf("Failed to load cluster information: %v", err)
				return fmt.Errorf("failed to load cluster information: %w", err)
			}
			launcher = cls.Spec.Launcher
			proxyMode = cls.Spec.ProxyMode
			k8sVersion = cls.Spec.K8sVersion
		}

		sshConfig, err := ssh.NewSSHConfig(password, sshKeyFile, sshPubKeyFile)
		if err != nil {
			return err
		}

		launcherType := myLauncher.LauncherType(launcher)
		if !launcherType.IsValid() {
			return fmt.Errorf("invalid launcher type: %s, currently only 'limactl' is supported", launcherType)
		}
		// Create environment initialization options
		// Create cluster configuration
		config := cluster.NewConfig(
			clusterName,
			workersCount,
			proxyMode,
			cluster.Resource{
				CPU:    masterCPU,
				Memory: masterMemory,
				Disk:   masterDisk,
			}, cluster.Resource{
				CPU:    workerCPU,
				Memory: workerMemory,
				Disk:   workerDisk,
			})
		config.SetKubernetesVersion(k8sVersion)
		config.SetLauncherType(launcherType.String())
		config.SetTemplate(limaTemplate)

		// Get default initialization options and modify required fields
		initOptions := initializer.DefaultInitOptions()
		initOptions.DisableSwap = !enableSwap // If enableSwap is true, DisableSwap is false
		initOptions.EnableIPVS = proxyMode == "ipvs"
		initOptions.K8SVersion = k8sVersion

		// Create cluster manager and pass options
		manager, err := manager.NewManager(config, sshConfig, cls)
		if err != nil {
			return err
		}

		// Set environment initialization options
		manager.SetInitOptions(initOptions)

		// Set CNI type
		manager.SetCNIType(cniType)

		// Set CSI type
		manager.SetCSIType(csiType)

		// Set whether to download kubeconfig
		manager.SetDownloadKubeconfig(downloadKubeconfig)

		// Set custom kubeadm config (if provided)
		if kubeadmConfigPath != "" {
			manager.SetKubeadmConfigPath(kubeadmConfigPath)
		}

		// Create cluster
		defer manager.Close()
		return manager.CreateCluster()
	},
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
	upCmd.Flags().StringVar(&cniType, "cni", "flannel", "CNI type to install (flannel, cilium, none)")
	upCmd.Flags().StringVar(&csiType, "csi", "local-path-provisioner", "CSI type to install (local-path-provisioner, rook-ceph, none)")
	upCmd.Flags().BoolVar(&downloadKubeconfig, "download-kubeconfig", true, "Download kubeconfig to local ~/.kube directory")
}
