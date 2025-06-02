package app

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/controller"
	"github.com/monshunter/ohmykube/pkg/log"
	myProvider "github.com/monshunter/ohmykube/pkg/provider"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Delete a k8s cluster",
	Long:  `Delete the created Kubernetes cluster and all related virtual machine resources`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sshConfig, err := ssh.NewSSHConfig(password, clusterName)
		if err != nil {
			return err
		}

		if provider == "" && config.CheckExists(clusterName) {
			clusterInfo, err := config.Load(clusterName)
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
		config := config.NewConfig(clusterName, workersCount, "iptables",
			config.Resource{}, config.Resource{})
		config.SetKubernetesVersion("")
		config.SetProvider(providerType.String())
		config.SetTemplate(template)

		// Create cluster manager
		manager, err := controller.NewManager(config, sshConfig, nil, nil)
		if err != nil {
			return err
		}
		defer manager.Close()
		// Delete cluster
		err = manager.DeleteCluster()
		if err != nil {
			return err
		}

		// Clean up SSH keys for the cluster
		if err := ssh.CleanupSSHKeys(clusterName); err != nil {
			log.Warningf("Failed to clean up SSH keys: %v", err)
		}

		return nil
	},
}

func init() {
	// No specific flags at the moment
}
