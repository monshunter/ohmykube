package cmd

import (
	"github.com/monshunter/ohmykube/pkg/cluster"
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
		// Create cluster configuration
		config := cluster.NewConfig(clusterName, k8sVersion, workersCount,
			cluster.Resource{
				CPU:    masterCPU,
				Memory: masterMemory,
				Disk:   masterDisk,
			}, cluster.Resource{
				CPU:    workerCPU,
				Memory: workerMemory,
				Disk:   workerDisk,
			})
		config.K8sVersion = k8sVersion
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
