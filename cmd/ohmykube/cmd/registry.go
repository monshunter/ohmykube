package cmd

import (
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/spf13/cobra"
)

var (
	harborMemory int
	harborCPU    int
	harborDisk   int
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "(Optional) Create a local harbor",
	Long:  `Create a local Harbor private container registry based on a virtual machine`,
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Info("Starting to create local Harbor instance...")
		log.Info("Note: This feature is not yet implemented and will be supported in future versions")
		// TODO: Implement Harbor creation logic
		// 1. Create VM
		// 2. Install Docker
		// 3. Install Harbor
		// 4. Configure certificates and access methods
		return nil
	},
}

func init() {
	registryCmd.Flags().IntVar(&harborMemory, "memory", 4096, "Harbor instance memory (MB)")
	registryCmd.Flags().IntVar(&harborCPU, "cpu", 2, "Harbor instance CPU cores")
	registryCmd.Flags().IntVar(&harborDisk, "disk", 40, "Harbor instance disk space (GB)")
}
