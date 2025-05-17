package cmd

import (
	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/environment"
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
	vmImage            string
	proxyMode          string
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
		sshConfig, err := ssh.NewSSHConfig(password, sshKeyFile, sshPubKeyFile)
		if err != nil {
			return err
		}
		// Create cluster configuration
		config := cluster.NewConfig(clusterName, k8sVersion, workersCount,
			cluster.Resource{
				CPU:    masterCPU,
				Memory: masterMemory,
				Disk:   masterDisk,
			}, cluster.Resource{
				CPU:    workerCPU,
				Memory: workerMemory,
				Disk:   workerDisk,
			})
		config.K8sVersion = k8sVersion

		// Create environment initialization options
		enableIPVS := proxyMode == "ipvs"

		// Get default initialization options and modify required fields
		initOptions := environment.DefaultInitOptions()
		initOptions.DisableSwap = !enableSwap // If enableSwap is true, DisableSwap is false
		initOptions.EnableIPVS = enableIPVS   // Enable IPVS when proxyMode is ipvs

		// Create cluster manager and pass options
		manager, err := manager.NewManager(config, sshConfig, nil)
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
		defer manager.CloseSSHClient()
		return manager.CreateCluster()
	},
}

func init() {
	// Add command line parameters
	upCmd.Flags().IntVarP(&workersCount, "workers", "w", 2, "Number of worker nodes")
	upCmd.Flags().IntVar(&masterMemory, "master-memory", 4096, "Master node memory (MB)")
	upCmd.Flags().IntVar(&masterCPU, "master-cpu", 2, "Master node CPU cores")
	upCmd.Flags().IntVar(&workerMemory, "worker-memory", 2048, "Worker node memory (MB)")
	upCmd.Flags().IntVar(&workerCPU, "worker-cpu", 1, "Worker node CPU cores")
	upCmd.Flags().IntVar(&masterDisk, "master-disk", 20, "Master node disk size (GB)")
	upCmd.Flags().IntVar(&workerDisk, "worker-disk", 10, "Worker node disk size (GB)")
	upCmd.Flags().StringVar(&k8sVersion, "k8s-version", "v1.33.0", "Kubernetes version")
	upCmd.Flags().StringVar(&vmImage, "vm-image", "24.04", "VM image")
	upCmd.Flags().StringVar(&proxyMode, "proxy-mode", "ipvs", "Proxy mode (iptables or ipvs)")
	upCmd.Flags().BoolVar(&enableSwap, "enable-swap", false, "Enable Swap (only for K8s 1.28+)")
	upCmd.Flags().StringVar(&kubeadmConfigPath, "kubeadm-config", "", "Custom kubeadm config file path")
	upCmd.Flags().StringVar(&cniType, "cni", "flannel", "CNI type to install (flannel, cilium, none)")
	upCmd.Flags().StringVar(&csiType, "csi", "local-path-provisioner", "CSI type to install (local-path-provisioner, rook-ceph, none)")
	upCmd.Flags().BoolVar(&downloadKubeconfig, "download-kubeconfig", true, "Download kubeconfig to local ~/.kube directory")
}
