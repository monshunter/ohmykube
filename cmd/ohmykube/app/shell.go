package app

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/controller"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell [nodeName]",
	Short: "Open an interactive shell to a virtual machine",
	Long:  `Open an interactive shell to a virtual machine. If no name is provided, connects to the master node.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName := args[0]
		// Load cluster information if it exists
		cls, err := config.Load(clusterName)
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
		config := &config.Config{
			Name:   cls.Name,
			Master: config.Resource{},
		}
		config.SetParallel(parallel)
		config.SetLauncherType(cls.Spec.Launcher)

		// Create cluster manager
		manager, err := controller.NewManager(config, sshConfig, cls, nil)
		if err != nil {
			log.Errorf("Failed to create manager: %v", err)
			return fmt.Errorf("failed to create manager: %w", err)
		}
		defer manager.Close()

		// Open shell
		log.Infof("Opening shell to VM: %s", nodeName)
		err = manager.OpenShell(nodeName)
		if err != nil {
			log.Errorf("Failed to open shell to VM %s: %v", nodeName, err)
			return fmt.Errorf("failed to open shell to VM %s: %w", nodeName, err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
}
