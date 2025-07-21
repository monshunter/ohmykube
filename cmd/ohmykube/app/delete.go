package app

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/controller"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	deleteForce bool
)

var deleteCmd = &cobra.Command{
	Use:     "delete",
	Aliases: []string{"del"},
	Short:   "Delete one or more nodes",
	Long:    `Delete one or more nodes from an existing Kubernetes cluster`,
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Set up graceful shutdown handling
		shutdownHandler := NewGracefulShutdownHandler()
		defer shutdownHandler.Close()

		// Get node names from args
		deleteNodeNames := args
		// Load cluster information
		cls, err := config.Load(clusterName)
		if err != nil {
			log.Errorf("Failed to load cluster information: %v", err)
			return fmt.Errorf("failed to load cluster information: %w", err)
		}

		// Read SSH configuration
		sshConfig, err := ssh.NewSSHConfig(password, clusterName)
		if err != nil {
			log.Errorf("Failed to create SSH configuration: %v", err)
			return fmt.Errorf("failed to create SSH configuration: %w", err)
		}

		// Create cluster configuration
		config := &config.Config{
			Name:   cls.Name,
			Master: config.Resource{},
		}
		config.SetParallel(parallel)
		config.SetProvider(cls.Spec.Provider)

		// Create cluster manager
		manager, err := controller.NewManager(config, sshConfig, cls, nil)
		if err != nil {
			log.Errorf("Failed to create cluster manager: %v", err)
			return fmt.Errorf("failed to create cluster manager: %w", err)
		}
		defer manager.Close()

		// Set manager for graceful shutdown
		shutdownHandler.SetManager(manager)

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
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Force delete the node even if it cannot be gracefully removed from the cluster")
}
