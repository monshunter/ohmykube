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
		// Load cluster information
		cls, err := config.Load(clusterName)
		if err != nil {
			log.Errorf("Failed to load cluster information: %v", err)
			return fmt.Errorf("failed to load cluster information: %w", err)
		}

		// Read SSH cfguration
		sshConfig, err := ssh.NewSSHConfig(password, sshKeyFile, sshPubKeyFile)
		if err != nil {
			log.Errorf("Failed to create SSH cfguration: %v", err)
			return fmt.Errorf("failed to create SSH cfguration: %w", err)
		}
		// Create cluster cfguration
		cfg := &config.Config{
			Name:   cls.Name,
			Master: config.Resource{},
		}
		cfg.SetKubernetesVersion(cls.Spec.K8sVersion)
		cfg.SetLauncherType(cls.Spec.Launcher)
		cfg.SetTemplate(limaTemplate)
		cfg.SetParallel(parallel)
		cfg.SetCNIType(cls.Spec.CNI)
		cfg.SetCSIType(cls.Spec.CSI)
		cfg.SetLBType(cls.Spec.LB)
		cfg.SetUpdateSystem(updateSystem)

		// Get default initialization options and modify required fields
		initOptions := initializer.DefaultInitOptions()
		initOptions.DisableSwap = !enableSwap // If enableSwap is true, DisableSwap is false
		initOptions.EnableIPVS = cls.Spec.ProxyMode == "ipvs"
		initOptions.K8SVersion = cls.Spec.K8sVersion
		initOptions.UpdateSystem = updateSystem

		// Create cluster manager
		manager, err := controller.NewManager(cfg, sshConfig, cls, nil)
		if err != nil {
			log.Errorf("Failed to create cluster manager: %v", err)
			return fmt.Errorf("failed to create cluster manager: %w", err)
		}
		defer manager.Close()
		// Set environment initialization options
		manager.SetInitOptions(initOptions)
		for range count {
			// Add node (InitOptions is already initialized in NewManager with DefaultInitOptions)
			if err := manager.AddWorkerNode(addNodeCPU, addNodeMemory, addNodeDisk); err != nil {
				log.Errorf("Failed to add node: %v", err)
				log.Errorf(`A failure has occurred.
			 You can either re-execute "ohmykube up" after the problem is fixed or directly. 
			 The process will resume from where it failed.`)
				return fmt.Errorf("failed to add node: %w", err)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().IntVar(&addNodeMemory, "memory", 2, "Node memory (GB)")
	addCmd.Flags().IntVar(&addNodeCPU, "cpu", 1, "Node CPU cores")
	addCmd.Flags().IntVar(&addNodeDisk, "disk", 10, "Node disk space (GB)")
	addCmd.Flags().IntVar(&count, "count", 1, "Number of nodes to add")
}
