package app

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/imageloader"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	targetNodes      []string
	containerRuntime string
	skipArchCheck    bool
)

var loadCmd = &cobra.Command{
	Use:   "load IMAGE_NAME",
	Short: "Load a local Docker image to cluster nodes",
	Long: `Load a local Docker image to cluster nodes, similar to 'kind load docker-image'.
This command exports the image from local Docker daemon and loads it to all cluster nodes
or specified nodes.

Examples:
  # Load image to all nodes in default cluster
  ohmykube load my-app:latest

  # Load image to specific nodes
  ohmykube load my-app:latest --nodes node1,node2

  # Use podman instead of docker
  ohmykube load my-app:latest --runtime podman

  # Skip architecture validation
  ohmykube load my-app:latest --skip-arch-check

  # Load image to specific cluster
  ohmykube load my-app:latest --name my-cluster`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		imageName := args[0]
		if imageName == "" {
			return fmt.Errorf("image name is required")
		}

		// Create container runtime first (fail fast if unsupported)
		runtime, err := imageloader.CreateContainerRuntime(containerRuntime)
		if err != nil {
			return fmt.Errorf("failed to initialize container runtime: %w", err)
		}

		log.Infof("🐳 Loading image '%s' to cluster '%s' using %s", imageName, clusterName, runtime.Name())

		// Check if cluster exists
		if !config.CheckExists(clusterName) {
			return fmt.Errorf("cluster '%s' does not exist. Create it first with 'ohmykube up'", clusterName)
		}

		// Load cluster configuration
		cluster, err := config.Load(clusterName)
		if err != nil {
			return fmt.Errorf("failed to load cluster configuration: %w", err)
		}

		// Create SSH manager
		sshConfig, err := ssh.NewSSHConfig(password, clusterName)
		if err != nil {
			return fmt.Errorf("failed to create SSH configuration: %w", err)
		}

		// Create image loader
		loader := imageloader.NewImageLoader(cluster, sshConfig, runtime)

		// Determine target nodes
		nodes, err := loader.GetTargetNodes(targetNodes)
		if err != nil {
			return fmt.Errorf("failed to get target nodes: %w", err)
		}

		if len(nodes) == 0 {
			return fmt.Errorf("no running nodes found in cluster '%s'", clusterName)
		}

		log.Infof("📦 Target nodes: %v", nodes)

		// Load image to nodes with architecture validation
		return loader.LoadImageToNodes(imageName, nodes, skipArchCheck)
	},
}

func init() {
	loadCmd.Flags().StringSliceVar(&targetNodes, "nodes", []string{},
		"Comma-separated list of node names to load the image to (default: all nodes)")
	loadCmd.Flags().StringVar(&containerRuntime, "runtime", "docker",
		"Container runtime to use (docker, podman, nerdctl)")
	loadCmd.Flags().BoolVar(&skipArchCheck, "skip-arch-check", false,
		"Skip architecture compatibility validation")
}
