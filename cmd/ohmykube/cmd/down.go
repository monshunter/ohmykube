package cmd

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/cluster"
	myLauncher "github.com/monshunter/ohmykube/pkg/launcher"
	"github.com/monshunter/ohmykube/pkg/manager"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Delete a k8s cluster",
	Long:  `Delete the created Kubernetes cluster and all related virtual machine resources`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sshConfig, err := ssh.NewSSHConfig(password, sshKeyFile, sshPubKeyFile)
		if err != nil {
			return err
		}

		if launcher == "" && cluster.CheckClusterInfomationExists(clusterName) {
			clusterInfo, err := cluster.LoadClusterInfomation(clusterName)
			if err != nil {
				return fmt.Errorf("failed to load cluster information: %w", err)
			}
			launcher = clusterInfo.Launcher
		}
		launcherType := myLauncher.LauncherType(launcher)
		if !launcherType.IsValid() {
			return fmt.Errorf("invalid launcher type: %s, please use %s or %s",
				launcher, myLauncher.MultipassLauncher, myLauncher.LimactlLauncher)
		}

		// Create cluster configuration
		config := cluster.NewConfig(clusterName, workersCount, "iptables",
			cluster.Resource{}, cluster.Resource{})
		config.SetKubernetesVersion("")
		config.SetLauncherType(launcherType.String())
		config.SetImage(multipassImage)
		config.SetTemplate(limaFile)

		// Create cluster manager
		manager, err := manager.NewManager(config, sshConfig, nil)
		if err != nil {
			return err
		}

		// Delete cluster
		return manager.DeleteCluster()
	},
}

func init() {
	// No specific flags at the moment
}
