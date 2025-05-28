package app

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/controller"
	myLauncher "github.com/monshunter/ohmykube/pkg/launcher"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	outputFormat string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List virtual machines",
	Long:  `List all virtual machines managed by OhMyKube. Supports various output formats.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load cluster information if it exists
		var cls *config.Cluster
		var err error
		if config.CheckExists(clusterName) {
			cls, err = config.Load(clusterName)
			if err != nil {
				log.Errorf("Failed to load cluster information: %v", err)
				return fmt.Errorf("failed to load cluster information: %w", err)
			}
			launcher = cls.Spec.Launcher
		}

		// Create SSH configuration
		sshConfig, err := ssh.NewSSHConfig(password, sshKeyFile, sshPubKeyFile)
		if err != nil {
			log.Errorf("Failed to create SSH configuration: %v", err)
			return fmt.Errorf("failed to create SSH configuration: %w", err)
		}

		// Validate launcher type
		launcherType := myLauncher.LauncherType(launcher)
		if !launcherType.IsValid() {
			return fmt.Errorf("invalid launcher type: %s, currently only 'limactl' is supported", launcher)
		}

		if outputFormat == "" {
			outputFormat = "table"
		}

		// Create manager
		manager, err := controller.NewManager(&config.Config{
			Name:         clusterName,
			LauncherType: launcher,
			Template:     limaTemplate,
			Parallel:     parallel,
			OutputFormat: outputFormat,
		}, sshConfig, cls, nil)
		if err != nil {
			log.Errorf("Failed to create manager: %v", err)
			return fmt.Errorf("failed to create manager: %w", err)
		}
		defer manager.Close()

		// List VMs
		vms, err := manager.ListVMs()
		if err != nil {
			log.Errorf("Failed to list VMs: %v", err)
			return fmt.Errorf("failed to list VMs: %w", err)
		}

		// Output results
		if len(vms) == 0 {
			fmt.Println("No virtual machines found")
		} else {
			if outputFormat == "yaml" {
				fmt.Println("---")
				for _, vm := range vms {
					fmt.Println(vm)
					fmt.Println("---")
				}
			} else {
				fmt.Print(strings.Join(vms, "\n"))
			}

		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().StringVar(&outputFormat, "output-format", "", "Output format (json, yaml, table)")
}
