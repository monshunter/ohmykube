package app

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/controller"
	"github.com/monshunter/ohmykube/pkg/initializer"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	addNodeMemory int
	addNodeCPU    int
	addNodeDisk   int
	count         int
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add one or more nodes",
	Long:  `Add one or more worker nodes to an existing Kubernetes cluster`,
	Args:  cobra.MinimumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Set up graceful shutdown handling
		shutdownHandler := NewGracefulShutdownHandler()
		defer shutdownHandler.Close()

		// Load cluster information
		cls, err := config.Load(clusterName)
		if err != nil {
			log.Errorf("Failed to load cluster information: %v", err)
			return fmt.Errorf("failed to load cluster information: %w", err)
		}

		// Read SSH configuration
		sshConfig, err := ssh.NewSSHConfig(password, clusterName)
		if err != nil {
			log.Errorf("Failed to create SSH configuration: %v", err)
			return fmt.Errorf("failed to create SSH configuration: %w", err)
		}
		// Create cluster cfguration
		cfg := &config.Config{
			Name:   cls.Name,
			Master: config.Resource{},
		}

		cfg.SetKubernetesVersion(cls.GetKubernetesVersion())
		cfg.SetProvider(cls.Spec.Provider)
		cfg.SetTemplate(template)
		cfg.SetParallel(parallel)
		cfg.SetCNIType(cls.GetCNI())
		cfg.SetCSIType(cls.GetCSI())
		cfg.SetLBType(cls.GetLoadBalancer())
		cfg.SetUpdateSystem(updateSystem)

		// Get default initialization options and modify required fields
		initOptions := initializer.DefaultInitOptions()
		initOptions.DisableSwap = !enableSwap // If enableSwap is true, DisableSwap is false
		initOptions.ProxyMode = initializer.ProxyMode(cls.GetProxyMode())
		initOptions.K8SVersion = cls.GetKubernetesVersion()
		initOptions.UpdateSystem = updateSystem

		// Create cluster manager
		manager, err := controller.NewManager(cfg, sshConfig, cls, nil)
		if err != nil {
			log.Errorf("Failed to create cluster manager: %v", err)
			return fmt.Errorf("failed to create cluster manager: %w", err)
		}
		defer manager.Close()

		// Set manager for graceful shutdown
		shutdownHandler.SetManager(manager)

		// Set environment initialization options
		manager.SetInitOptions(initOptions)

		// Check for incomplete node additions and resume them first
		if err := manager.ResumeIncompleteWorkerNodes(); err != nil {
			log.Errorf("Failed to resume incomplete worker nodes: %v", err)
			return fmt.Errorf("failed to resume incomplete worker nodes: %w", err)
		}

		// Add new nodes as requested
		if err := manager.AddWorkerNodes(addNodeCPU, addNodeMemory, addNodeDisk, count); err != nil {
			log.Errorf("Failed to add node: %v", err)
			log.Errorf(`A failure has occurred.
			 You can either re-execute "ohmykube add" after the problem is fixed.
			 The process will resume from where it failed.`)
			return fmt.Errorf("failed to add node: %w", err)
		}
		return nil
	},
}

func init() {
	addCmd.Flags().IntVar(&addNodeMemory, "memory", 2, "Node memory (GB)")
	addCmd.Flags().IntVar(&addNodeCPU, "cpu", 1, "Node CPU cores")
	addCmd.Flags().IntVar(&addNodeDisk, "disk", 10, "Node disk space (GB)")
	addCmd.Flags().IntVar(&count, "count", 1, "Number of nodes to add")
}
