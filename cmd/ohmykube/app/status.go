package app

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/spf13/cobra"
)

var (
	wideOutput bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all clusters",
	Long:  `Show detailed status information for all clusters, including the current default cluster.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get all clusters
		clusters, err := config.ListAllClusters()
		if err != nil {
			return fmt.Errorf("failed to list clusters: %w", err)
		}

		if len(clusters) == 0 {
			fmt.Println("No clusters found")
			return nil
		}

		// Get current cluster
		currentCluster, err := config.GetCurrentCluster()
		if err != nil {
			log.Warningf("Failed to get current cluster: %v", err)
			currentCluster = "" // Set to empty if we can't determine
		}

		// Print header and rows based on output mode
		if wideOutput {
			// Wide output mode
			fmt.Println("CLUSTERS:")
			fmt.Println(strings.Repeat("-", 150))
			fmt.Printf("%-15s %-10s %-15s %-8s %-10s %-10s %-10s %-26s %-15s %s\n",
				"NAME", "STATUS", "K8S-VERSION", "NODES", "RUNNING", "UPTIME", "CNI", "CSI", "LOAD-BALANCER", "PATH")
			fmt.Println(strings.Repeat("-", 150))

			for _, clusterName := range clusters {
				clusterInfo, err := config.GetClusterInfo(clusterName)
				if err != nil {
					log.Errorf("Failed to get info for cluster '%s': %v", clusterName, err)
					continue
				}

				loadBalancer := clusterInfo.LoadBalancer
				if loadBalancer == "" {
					loadBalancer = "none"
				}

				fmt.Printf("%-15s %-10s %-15s %-8s %-10s %-10s %-10s %-26s %-15s %s\n",
					clusterInfo.Name,
					string(clusterInfo.Status),
					clusterInfo.KubernetesVersion,
					fmt.Sprintf("%d", clusterInfo.Nodes),
					fmt.Sprintf("%d/%d", clusterInfo.RunningNodes, clusterInfo.Nodes),
					clusterInfo.Uptime,
					clusterInfo.CNI,
					clusterInfo.CSI,
					loadBalancer,
					clusterInfo.FilePath)
			}

			fmt.Println(strings.Repeat("-", 150))
		} else {
			// Standard output mode
			fmt.Println("CLUSTERS:")
			fmt.Println(strings.Repeat("-", 80))
			fmt.Printf("%-15s %-10s %-15s %-8s %-10s %s\n",
				"NAME", "STATUS", "K8S-VERSION", "NODES", "RUNNING", "UPTIME")
			fmt.Println(strings.Repeat("-", 80))

			for _, clusterName := range clusters {
				clusterInfo, err := config.GetClusterInfo(clusterName)
				if err != nil {
					log.Errorf("Failed to get info for cluster '%s': %v", clusterName, err)
					continue
				}

				fmt.Printf("%-15s %-10s %-15s %-8s %-10s %s\n",
					clusterInfo.Name,
					string(clusterInfo.Status),
					clusterInfo.KubernetesVersion,
					fmt.Sprintf("%d", clusterInfo.Nodes),
					fmt.Sprintf("%d/%d", clusterInfo.RunningNodes, clusterInfo.Nodes),
					clusterInfo.Uptime)
			}

			fmt.Println(strings.Repeat("-", 80))
		}
		fmt.Printf("Total clusters: %d\n", len(clusters))
		if currentCluster != "" {
			fmt.Printf("Current cluster: %s\n", currentCluster)
		}

		return nil
	},
}

func init() {
	statusCmd.Flags().BoolVarP(&wideOutput, "wide", "w", false, "Show additional columns (CNI, CSI, Load Balancer, Path)")
}
