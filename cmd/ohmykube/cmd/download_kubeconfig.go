package cmd

import (
	"fmt"
	"os"

	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/kubeconfig"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var downloadKubeconfigCmd = &cobra.Command{
	Use:   "download-kubeconfig",
	Short: "Download cluster kubeconfig to local",
	Long:  `Download the current cluster's kubeconfig file to the local ~/.kube directory for local debugging and cluster management`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load cluster information
		clusterInfo, err := cluster.Load(clusterName)
		if err != nil {
			log.Errorf("Failed to load cluster information: %v", err)
			return fmt.Errorf("failed to load cluster information: %w", err)
		}

		// Check master node information
		if clusterInfo.Master.Status.IP == "" {
			log.Errorf("Unable to get master node IP address")
			return fmt.Errorf("unable to get master node IP address")
		}

		// Read SSH key
		sshKeyContent, err := os.ReadFile(sshKeyFile)
		if err != nil {
			log.Errorf("Failed to read SSH key: %v", err)
			return fmt.Errorf("failed to read SSH key: %w", err)
		}

		// Create SSH client
		sshClient := ssh.NewClient(
			clusterInfo.Master.Status.IP,
			clusterInfo.Auth.Port,
			clusterInfo.Auth.User,
			password,
			string(sshKeyContent),
		)

		// Connect to SSH server
		if err := sshClient.Connect(); err != nil {
			log.Errorf("Failed to connect to master node: %v", err)
			return fmt.Errorf("failed to connect to master node: %w", err)
		}
		defer sshClient.Close()

		// Use the unified kubeconfig download function
		kubeconfigPath, err := kubeconfig.DownloadToLocal(sshClient, clusterInfo.Name, "")
		if err != nil {
			log.Errorf("Failed to download kubeconfig: %v", err)
			return fmt.Errorf("failed to download kubeconfig: %w", err)
		}

		log.Infof("Kubeconfig has been downloaded to: %s", kubeconfigPath)
		log.Info("You can access the cluster with the following commands:")
		log.Infof("export KUBECONFIG=%s", kubeconfigPath)
		log.Info("kubectl get nodes")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(downloadKubeconfigCmd)
}
