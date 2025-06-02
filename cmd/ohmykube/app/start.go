package app

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/controller"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start [nodeName]",
	Short: "Start a virtual machine",
	Long:  `Start a virtual machine by name. If no name is provided, starts all VMs in the cluster.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load cluster information if it exists
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
			log.Errorf("Failed to create manager: %v", err)
			return fmt.Errorf("failed to create manager: %w", err)
		}
		defer manager.Close()

		// Start specific VM
		for _, nodeName := range args {
			err = manager.StartVM(nodeName)
			if err != nil {
				log.Errorf("Failed to start VM %s: %v", nodeName, err)
				return fmt.Errorf("failed to start VM %s: %w", nodeName, err)
			}
		}
		return nil
	},
}
