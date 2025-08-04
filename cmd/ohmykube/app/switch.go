package app

import (
	"fmt"
	"path/filepath"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/spf13/cobra"
)

// showKubeconfigGuidance displays instructions for updating KUBECONFIG
func showKubeconfigGuidance(clusterName string) {
	// Path to the cluster's kubeconfig
	clusterDir := filepath.Join("~", ".ohmykube", clusterName)
	kubeconfigPath := filepath.Join(clusterDir, "kubeconfig")

	fmt.Println("\nTo update your KUBECONFIG for kubectl access, run one of the following:")
	fmt.Printf("  export KUBECONFIG=%s\n", kubeconfigPath)
	fmt.Println("  # OR")
	fmt.Printf("  kubectl config use-context ohmykube-%s\n", clusterName)
	fmt.Println("  # OR copy to default location:")
	fmt.Printf("  cp %s ~/.kube/config\n", kubeconfigPath)
	fmt.Println("\nNote: Make sure to backup your existing ~/.kube/config before copying!")
}

var switchCmd = &cobra.Command{
	Use:   "switch [cluster-name]",
	Short: "Switch to a different cluster",
	Long:  `Switch the current default cluster context to the specified cluster.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetCluster := args[0]

		// Check if the target cluster exists
		if !config.CheckExists(targetCluster) {
			return fmt.Errorf("cluster '%s' does not exist", targetCluster)
		}

		// Get the current cluster for comparison
		currentCluster, err := config.GetCurrentCluster()
		if err != nil {
			log.Warningf("Failed to get current cluster: %v, proceeding with switch", err)
		}

		// Check if already switched to this cluster
		if currentCluster == targetCluster {
			fmt.Printf("Already switched to cluster '%s'\n", targetCluster)
			return nil
		}

		// Switch to the target cluster
		err = config.SetCurrentCluster(targetCluster)
		if err != nil {
			return fmt.Errorf("failed to switch to cluster '%s': %w", targetCluster, err)
		}

		fmt.Printf("Switched to cluster '%s'\n", targetCluster)

		// Show KUBECONFIG guidance
		showKubeconfigGuidance(targetCluster)

		return nil
	},
}
