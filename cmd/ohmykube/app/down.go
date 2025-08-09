package app

import (
	"fmt"

	configpkg "github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/controller"
	"github.com/monshunter/ohmykube/pkg/log"
	myProvider "github.com/monshunter/ohmykube/pkg/provider"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	downConfigFile string
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Delete a k8s cluster",
	Long:  `Delete the created Kubernetes cluster and all related virtual machine resources`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Set up graceful shutdown handling
		shutdownHandler := NewGracefulShutdownHandler()
		defer shutdownHandler.Close()

		// Load cluster from config file if specified
		if downConfigFile != "" {
			log.Infof("📄 Loading cluster configuration from file: %s", downConfigFile)
			loadedCluster, err := configpkg.LoadClusterFromFile(downConfigFile)
			if err != nil {
				return fmt.Errorf("failed to load cluster from config file: %w", err)
			}
			// Set cluster name from loaded configuration
			clusterName = loadedCluster.Name
			log.Infof("🏷️  Using cluster name from config file: %s", clusterName)
		}

		sshConfig, err := ssh.NewSSHConfig(password, clusterName)
		if err != nil {
			return err
		}

		if provider == "" && configpkg.CheckExists(clusterName) {
			clusterInfo, err := configpkg.LoadCluster(clusterName)
			if err != nil {
				return fmt.Errorf("failed to load cluster information: %w", err)
			}
			provider = clusterInfo.Spec.Provider
		}
		providerType := myProvider.ProviderType(provider)
		if !providerType.IsValid() {
			return fmt.Errorf("invalid provider type: %s, currently only 'lima' is supported", provider)
		}

		// Create cluster configuration
		config := configpkg.NewConfig(clusterName, workersCount, "iptables",
			configpkg.Resource{}, configpkg.Resource{})
		config.SetKubernetesVersion("")
		config.SetProvider(providerType.String())
		config.SetTemplate(template)

		// Create cluster manager
		manager, err := controller.NewManager(config, sshConfig, nil, nil)
		if err != nil {
			return err
		}
		defer manager.Close()

		// Set manager for graceful shutdown
		shutdownHandler.SetManager(manager)

		// Delete cluster
		err = manager.DeleteCluster()
		if err != nil {
			return err
		}

		// Clean up SSH keys for the cluster
		if err := ssh.CleanupSSHKeys(clusterName); err != nil {
			log.Warningf("Failed to clean up SSH keys: %v", err)
		}

		// Auto-switch to next available cluster if current cluster was deleted
		if err := configpkg.AutoSwitchAfterDeletion(clusterName); err != nil {
			log.Warningf("Failed to auto-switch cluster: %v", err)
		}

		return nil
	},
}

func init() {
	downCmd.Flags().StringVarP(&downConfigFile, "file", "f", "", "Path to cluster configuration file")
}
