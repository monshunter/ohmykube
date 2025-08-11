package app

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/spf13/cobra"
)

var addonCmd = &cobra.Command{
	Use:   "addon",
	Short: "Manage cluster addons",
	Long:  "Install, upgrade, remove and manage cluster addons",
}

var addonListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all addons and their status",
	Args:  cobra.ExactArgs(0),
	RunE:  runAddonList,
}

var addonStatusCmd = &cobra.Command{
	Use:   "status [addon-name]",
	Short: "Show detailed status of a specific addon",
	Args:  cobra.ExactArgs(1),
	RunE:  runAddonStatus,
}

func init() {
	rootCmd.AddCommand(addonCmd)
	addonCmd.AddCommand(addonListCmd)
	addonCmd.AddCommand(addonStatusCmd)
}

func runAddonList(cmd *cobra.Command, args []string) error {
	cluster := getCurrentCluster()
	if cluster == nil {
		return fmt.Errorf("no active cluster found")
	}

	statuses := cluster.ListAddonStatuses()
	if len(statuses) == 0 {
		fmt.Println("No addons installed")
		return nil
	}

	// Print addon table
	printAddonTable(statuses)
	return nil
}

func runAddonStatus(cmd *cobra.Command, args []string) error {
	addonName := args[0]
	cluster := getCurrentCluster()
	if cluster == nil {
		return fmt.Errorf("no active cluster found")
	}

	status, found := cluster.GetAddonStatus(addonName)
	if !found {
		return fmt.Errorf("addon '%s' not found", addonName)
	}

	// Print detailed addon status
	printAddonDetails(*status)
	return nil
}

func getCurrentCluster() *config.Cluster {
	// Get current cluster name
	currentClusterName, err := config.GetCurrentCluster()
	if err != nil {
		return nil
	}

	// Load current cluster
	cluster, err := config.LoadCluster(currentClusterName)
	if err != nil {
		return nil
	}

	return cluster
}

func printAddonTable(statuses []config.AddonStatus) {
	fmt.Printf("%-20s %-12s %-15s %-15s %-20s\n", "NAME", "PHASE", "INSTALLED", "DESIRED", "LAST UPDATE")
	fmt.Println(strings.Repeat("-", 90))

	for _, status := range statuses {
		lastUpdate := "Never"
		if status.LastUpdateTime != nil {
			lastUpdate = status.LastUpdateTime.Format("2006-01-02 15:04:05")
		}

		fmt.Printf("%-20s %-12s %-15s %-15s %-20s\n",
			status.Name,
			status.Phase,
			status.InstalledVersion,
			status.DesiredVersion,
			lastUpdate,
		)
	}
}

func printAddonDetails(status config.AddonStatus) {
	fmt.Printf("Name:             %s\n", status.Name)
	fmt.Printf("Phase:            %s\n", status.Phase)
	fmt.Printf("Installed Version:%s\n", status.InstalledVersion)
	fmt.Printf("Desired Version:  %s\n", status.DesiredVersion)
	fmt.Printf("Namespace:        %s\n", status.Namespace)

	if status.InstallTime != nil {
		fmt.Printf("Install Time:     %s\n", status.InstallTime.Format("2006-01-02 15:04:05"))
	}

	if status.LastUpdateTime != nil {
		fmt.Printf("Last Update:      %s\n", status.LastUpdateTime.Format("2006-01-02 15:04:05"))
	}

	fmt.Printf("Message:          %s\n", status.Message)
	fmt.Printf("Reason:           %s\n", status.Reason)

	// Print cached images if any
	if len(status.Images) > 0 {
		fmt.Println("\nCached Images:")
		for _, image := range status.Images {
			fmt.Printf("  - %s\n", image)
		}
	}

	// Print tracked resources if any
	if len(status.Resources) > 0 {
		fmt.Println("\nTracked Resources:")
		for _, resource := range status.Resources {
			fmt.Printf("  - %s/%s %s (ns: %s)\n", resource.APIVersion, resource.Kind, resource.Name, resource.Namespace)
		}
	}

	// Print conditions if any
	if len(status.Conditions) > 0 {
		fmt.Println("\nConditions:")
		for _, condition := range status.Conditions {
			fmt.Printf("  - Type: %s, Status: %s, Reason: %s\n",
				condition.Type, condition.Status, condition.Reason)
			if condition.Message != "" {
				fmt.Printf("    Message: %s\n", condition.Message)
			}
		}
	}
}
