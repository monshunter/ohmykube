package cmd

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/initializer"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/manager"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	addNodeMemory int
	addNodeCPU    int
	addNodeDisk   int
	count         int
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add one or more nodes",
	Long:  `Add one or more worker nodes to an existing Kubernetes cluster`,
	Args:  cobra.MinimumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load cluster information
		cls, err := cluster.Load(clusterName)
		if err != nil {
			log.Errorf("Failed to load cluster information: %v", err)
			return fmt.Errorf("failed to load cluster information: %w", err)
		}

		// Read SSH configuration
		sshConfig, err := ssh.NewSSHConfig(password, sshKeyFile, sshPubKeyFile)
		if err != nil {
			log.Errorf("Failed to create SSH configuration: %v", err)
			return fmt.Errorf("failed to create SSH configuration: %w", err)
		}
		// Create cluster configuration
		config := &cluster.Config{
			Name:   cls.Name,
			Master: cluster.Resource{},
		}
		config.SetKubernetesVersion(cls.Spec.K8sVersion)
		config.SetLauncherType(cls.Spec.Launcher)
		config.SetTemplate(limaTemplate)
		config.SetParallel(parallel)
		// Get default initialization options and modify required fields
		initOptions := initializer.DefaultInitOptions()
		initOptions.DisableSwap = !enableSwap // If enableSwap is true, DisableSwap is false
		initOptions.EnableIPVS = cls.Spec.ProxyMode == "ipvs"
		initOptions.K8SVersion = cls.Spec.K8sVersion

		// Create cluster manager
		manager, err := manager.NewManager(config, sshConfig, cls)
		if err != nil {
			log.Errorf("Failed to create cluster manager: %v", err)
			return fmt.Errorf("failed to create cluster manager: %w", err)
		}
		defer manager.Close()
		// Set environment initialization options
		manager.SetInitOptions(initOptions)
		for range count {
			// Add node (InitOptions is already initialized in NewManager with DefaultInitOptions)
			if err := manager.AddWorkerNode(addNodeCPU, addNodeMemory, addNodeDisk); err != nil {
				log.Errorf("Failed to add node: %v", err)
				return fmt.Errorf("failed to add node: %w", err)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().IntVar(&addNodeMemory, "memory", 2, "Node memory (GB)")
	addCmd.Flags().IntVar(&addNodeCPU, "cpu", 1, "Node CPU cores")
	addCmd.Flags().IntVar(&addNodeDisk, "disk", 10, "Node disk space (GB)")
	addCmd.Flags().IntVar(&count, "count", 1, "Number of nodes to add")
}
