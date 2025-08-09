package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/controller"
	"github.com/monshunter/ohmykube/pkg/initializer"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	addNodeMemory      int
	addNodeCPU         int
	addNodeDisk        int
	count              int
	addNodeLabels      []string
	addNodeAnnotations []string
	addNodeTaints      []string

	// Custom initialization parameters
	hookPreSystemInit  []string
	hookPostSystemInit []string
	hookPreK8sInit     []string
	hookPostK8sInit    []string
	uploadFiles        []string
	uploadDirs         []string
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
		cls, err := config.LoadCluster(clusterName)
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
		cfg.SetUpdateSystem(cls.GetUpdateSystem())

		// Parse and set node metadata for new nodes
		nodeMetadata, err := parseNodeMetadata(addNodeLabels, addNodeAnnotations, addNodeTaints)
		if err != nil {
			return fmt.Errorf("failed to parse node metadata: %w", err)
		}
		cfg.SetWorkerMetadata(nodeMetadata)

		// Parse and set custom initialization configuration
		customInit, err := parseCustomInitConfig(hookPreSystemInit, hookPostSystemInit, hookPreK8sInit, hookPostK8sInit, uploadFiles, uploadDirs)
		if err != nil {
			return fmt.Errorf("failed to parse custom initialization config: %w", err)
		}
		cfg.SetWorkerCustomInit(customInit)

		// Get default initialization options and modify required fields
		initOptions := initializer.DefaultInitOptions()
		initOptions.DisableSwap = !enableSwap // If enableSwap is true, DisableSwap is false
		initOptions.ProxyMode = initializer.ProxyMode(cls.GetProxyMode())
		initOptions.K8SVersion = cls.GetKubernetesVersion()
		initOptions.UpdateSystem = cls.GetUpdateSystem()

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

		// Add new nodes as requested
		if err := manager.AddWorkerNodes(addNodeCPU, addNodeMemory, addNodeDisk, count); err != nil {
			log.Errorf("Failed to add node: %v", err)
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

	// Node metadata flags
	addCmd.Flags().StringArrayVar(&addNodeLabels, "labels", []string{},
		`Labels for the new node(s) (format: key=value). Can be specified multiple times`)
	addCmd.Flags().StringArrayVar(&addNodeAnnotations, "annotations", []string{},
		`Annotations for the new node(s) (format: key=value). Can be specified multiple times`)
	addCmd.Flags().StringArrayVar(&addNodeTaints, "taints", []string{},
		`Taints for the new node(s) (format: key=value:effect). Can be specified multiple times. Effect can be NoSchedule|NoExecute|PreferNoSchedule`)

	// Custom initialization flags
	addCmd.Flags().StringArrayVar(&hookPreSystemInit, "hook-pre-system-init", []string{},
		`Hook scripts to run before system update (format: /path/to/script.sh). Multiple files supported with comma separation`)
	addCmd.Flags().StringArrayVar(&hookPostSystemInit, "hook-post-system-init", []string{},
		`Hook scripts to run after system update, before K8s install (format: /path/to/script.sh). Multiple files supported with comma separation`)
	addCmd.Flags().StringArrayVar(&hookPreK8sInit, "hook-pre-k8s-init", []string{},
		`Hook scripts to run before K8s components install (format: /path/to/script.sh). Multiple files supported with comma separation`)
	addCmd.Flags().StringArrayVar(&hookPostK8sInit, "hook-post-k8s-init", []string{},
		`Hook scripts to run after all initialization (format: /path/to/script.sh). Multiple files supported with comma separation`)
	addCmd.Flags().StringArrayVar(&uploadFiles, "upload-file", []string{},
		`Files to upload to nodes (format: local_path:remote_path[:mode[:owner]]). Example: /tmp/config.yml:/etc/config.yml:0644:root:root`)
	addCmd.Flags().StringArrayVar(&uploadDirs, "upload-dir", []string{},
		`Directories to upload to nodes (format: local_dir:remote_dir[:mode[:owner]]). Recursively copies entire directory`)
}

// parseCustomInitConfig parses custom initialization configuration from command line arguments
func parseCustomInitConfig(preSystemInit, postSystemInit, preK8sInit, postK8sInit, uploadFiles, uploadDirs []string) (config.CustomInitConfig, error) {
	customInit := config.CustomInitConfig{
		Hooks: config.CustomInitHooks{},
		Files: []config.FileUpload{},
	}

	// Parse hook scripts for each phase
	if err := parseHookScripts(&customInit.Hooks.PreSystemInit, preSystemInit); err != nil {
		return customInit, fmt.Errorf("failed to parse pre-system-init hooks: %w", err)
	}
	if err := parseHookScripts(&customInit.Hooks.PostSystemInit, postSystemInit); err != nil {
		return customInit, fmt.Errorf("failed to parse post-system-init hooks: %w", err)
	}
	if err := parseHookScripts(&customInit.Hooks.PreK8sInit, preK8sInit); err != nil {
		return customInit, fmt.Errorf("failed to parse pre-k8s-init hooks: %w", err)
	}
	if err := parseHookScripts(&customInit.Hooks.PostK8sInit, postK8sInit); err != nil {
		return customInit, fmt.Errorf("failed to parse post-k8s-init hooks: %w", err)
	}

	// Parse file uploads
	for _, fileSpec := range uploadFiles {
		fileUpload, err := parseFileUploadSpec(fileSpec, false)
		if err != nil {
			return customInit, fmt.Errorf("failed to parse file upload spec '%s': %w", fileSpec, err)
		}
		customInit.Files = append(customInit.Files, fileUpload)
	}

	// Parse directory uploads (marked as directories for special handling)
	for _, dirSpec := range uploadDirs {
		dirUpload, err := parseFileUploadSpec(dirSpec, true)
		if err != nil {
			return customInit, fmt.Errorf("failed to parse directory upload spec '%s': %w", dirSpec, err)
		}
		customInit.Files = append(customInit.Files, dirUpload)
	}

	return customInit, nil
}

// parseHookScripts parses hook script file paths and reads their contents
func parseHookScripts(target *[]string, scriptPaths []string) error {
	for _, pathList := range scriptPaths {
		// Support comma-separated file paths
		paths := strings.Split(pathList, ",")
		for _, path := range paths {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}

			// Check if file exists
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("script file '%s' does not exist: %w", path, err)
			}

			// Read script content
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read script file '%s': %w", path, err)
			}

			// Add script content to target slice
			*target = append(*target, string(content))
		}
	}
	return nil
}

// parseFileUploadSpec parses file upload specification in format: local_path:remote_path[:mode[:owner]]
func parseFileUploadSpec(spec string, isDirectory bool) (config.FileUpload, error) {
	upload := config.FileUpload{
		Mode:  "0644",      // Default mode
		Owner: "root:root", // Default owner
	}

	parts := strings.Split(spec, ":")
	if len(parts) < 2 {
		return upload, fmt.Errorf("invalid format, expected 'local:remote[:mode[:owner]]', got '%s'", spec)
	}

	upload.LocalPath = strings.TrimSpace(parts[0])
	upload.RemotePath = strings.TrimSpace(parts[1])

	// Validate local path exists
	if _, err := os.Stat(upload.LocalPath); err != nil {
		return upload, fmt.Errorf("local path '%s' does not exist: %w", upload.LocalPath, err)
	}

	// Check if it's a directory when expected
	stat, err := os.Stat(upload.LocalPath)
	if err != nil {
		return upload, fmt.Errorf("failed to stat local path '%s': %w", upload.LocalPath, err)
	}

	if isDirectory && !stat.IsDir() {
		return upload, fmt.Errorf("expected directory but '%s' is not a directory", upload.LocalPath)
	}

	if !isDirectory && stat.IsDir() {
		return upload, fmt.Errorf("expected file but '%s' is a directory, use --upload-dir instead", upload.LocalPath)
	}

	// Parse optional mode
	if len(parts) > 2 && strings.TrimSpace(parts[2]) != "" {
		mode := strings.TrimSpace(parts[2])
		// Validate octal mode
		if _, err := strconv.ParseInt(mode, 8, 32); err != nil {
			return upload, fmt.Errorf("invalid file mode '%s', expected octal format like '0644'", mode)
		}
		upload.Mode = mode
	}

	// Parse optional owner
	if len(parts) > 3 && strings.TrimSpace(parts[3]) != "" {
		upload.Owner = strings.TrimSpace(strings.Join(parts[3:], ":"))
		// Basic validation for owner format (user:group)
		if !strings.Contains(upload.Owner, ":") || strings.Count(upload.Owner, ":") > 1 {
			return upload, fmt.Errorf("invalid owner format '%s', expected 'user:group' format", upload.Owner)
		}
	}

	return upload, nil
}
