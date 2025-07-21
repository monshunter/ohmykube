package app

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/controller"
	"github.com/monshunter/ohmykube/pkg/log"
	myProvider "github.com/monshunter/ohmykube/pkg/provider"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	outputFormat string
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List virtual machines",
	Long:    `List all virtual machines managed by OhMyKube. Supports various output formats.`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Set up graceful shutdown handling
		shutdownHandler := NewGracefulShutdownHandler()
		defer shutdownHandler.Close()

		// Load cluster information if it exists
		var cls *config.Cluster
		var err error
		if config.CheckExists(clusterName) {
			cls, err = config.Load(clusterName)
			if err != nil {
				log.Errorf("Failed to load cluster information: %v", err)
				return fmt.Errorf("failed to load cluster information: %w", err)
			}
			provider = cls.Spec.Provider
		}

		// Create SSH configuration
		sshConfig, err := ssh.NewSSHConfig(password, clusterName)
		if err != nil {
			log.Errorf("Failed to create SSH configuration: %v", err)
			return fmt.Errorf("failed to create SSH configuration: %w", err)
		}

		// Validate provider type
		providerType := myProvider.ProviderType(provider)
		if !providerType.IsValid() {
			return fmt.Errorf("invalid provider type: %s, currently only 'lima' is supported", provider)
		}

		if outputFormat == "" {
			outputFormat = "table"
		}

		// Create manager
		manager, err := controller.NewManager(&config.Config{
			Name:         clusterName,
			Provider:     provider,
			Template:     template,
			Parallel:     parallel,
			OutputFormat: outputFormat,
		}, sshConfig, cls, nil)
		if err != nil {
			log.Errorf("Failed to create manager: %v", err)
			return fmt.Errorf("failed to create manager: %w", err)
		}
		defer manager.Close()

		// Set manager for graceful shutdown
		shutdownHandler.SetManager(manager)

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
	listCmd.Flags().StringVar(&outputFormat, "output-format", "", "Output format (json, yaml, table)")
}
