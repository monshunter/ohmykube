package cmd

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/manager"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	addNodeMemory int
	addNodeCPU    int
	addNodeDisk   int
	addNodeRole   string
	count         int
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add one or more nodes",
	Long:  `Add one or more worker nodes to an existing Kubernetes cluster`,
	Args:  cobra.MinimumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load cluster information
		clusterInfo, err := cluster.LoadClusterInfomation(clusterName)
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
			Name:       clusterInfo.Name,
			K8sVersion: clusterInfo.K8sVersion,
			Master:     cluster.Node{Name: clusterInfo.Master.Name},
		}

		// Create cluster manager
		manager, err := manager.NewManager(config, sshConfig, clusterInfo)
		if err != nil {
			log.Errorf("Failed to create cluster manager: %v", err)
			return fmt.Errorf("failed to create cluster manager: %w", err)
		}
		defer manager.CloseSSHClient()
		for range count {
			// Add node (InitOptions is already initialized in NewManager with DefaultInitOptions)
			if err := manager.AddNode(addNodeRole, addNodeCPU, addNodeMemory, addNodeDisk); err != nil {
				log.Errorf("Failed to add node: %v", err)
				return fmt.Errorf("failed to add node: %w", err)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().IntVar(&addNodeMemory, "memory", 2048, "Node memory (MB)")
	addCmd.Flags().IntVar(&addNodeCPU, "cpu", 2, "Node CPU cores")
	addCmd.Flags().IntVar(&addNodeDisk, "disk", 20, "Node disk space (GB)")
	addCmd.Flags().StringVar(&addNodeRole, "role", "worker", "Node role (worker/master)")
	addCmd.Flags().IntVar(&count, "count", 1, "Number of nodes to add")
}
