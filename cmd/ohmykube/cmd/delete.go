package cmd

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/manager"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	deleteForce bool
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete one or more nodes",
	Long:  `Delete one or more nodes from an existing Kubernetes cluster`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get node names from args
		deleteNodeNames := args
		// Load cluster information
		clusterInfo, err := cluster.Load(clusterName)
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
			Name:   clusterInfo.Name,
			Master: cluster.Resource{},
		}
		config.SetParallel(parallel)
		config.SetLauncherType(clusterInfo.Launcher)

		// Create cluster manager
		manager, err := manager.NewManager(config, sshConfig, clusterInfo)
		if err != nil {
			log.Errorf("Failed to create cluster manager: %v", err)
			return fmt.Errorf("failed to create cluster manager: %w", err)
		}
		defer manager.Close()
		// Delete nodes
		for _, nodeName := range deleteNodeNames {
			nodeName = strings.TrimSpace(nodeName)
			if err := manager.DeleteNode(nodeName, deleteForce); err != nil {
				log.Errorf("Failed to delete node %s: %v", nodeName, err)
				return fmt.Errorf("failed to delete node %s: %w", nodeName, err)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Force delete the node even if it cannot be gracefully removed from the cluster")
}
