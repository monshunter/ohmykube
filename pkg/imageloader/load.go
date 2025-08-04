package imageloader

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// ContainerRuntime represents different container runtime tools
type ContainerRuntime string

const (
	DockerRuntimeType  ContainerRuntime = "docker"
	PodmanRuntimeType  ContainerRuntime = "podman"
	NerdctlRuntimeType ContainerRuntime = "nerdctl"
)

// ContainerRuntimeInterface abstracts container operations
type ContainerRuntimeInterface interface {
	Name() string
	IsAvailable() bool
	ExportImage(imageName, outputPath string) error
	InspectImage(imageName string) (*ImageInfo, error)
}

// ImageInfo contains image metadata
type ImageInfo struct {
	Architecture string
	Platform     string
	Size         int64
}

// ImageLoader handles loading container images to cluster nodes
type ImageLoader struct {
	cluster          *config.Cluster
	sshConfig        *ssh.SSHConfig
	containerRuntime ContainerRuntimeInterface
}

func NewImageLoader(cluster *config.Cluster, sshConfig *ssh.SSHConfig, runtime ContainerRuntimeInterface) *ImageLoader {
	return &ImageLoader{
		cluster:          cluster,
		sshConfig:        sshConfig,
		containerRuntime: runtime,
	}
}

// getTargetNodes returns the list of nodes to load the image to
func (l *ImageLoader) GetTargetNodes(specifiedNodes []string) ([]string, error) {
	allNodeIPs := l.cluster.Nodes2IPsMap()

	if len(specifiedNodes) > 0 {
		// Validate specified nodes exist
		var validNodes []string
		for _, nodeName := range specifiedNodes {
			if _, exists := allNodeIPs[nodeName]; exists {
				validNodes = append(validNodes, nodeName)
			} else {
				log.Warningf("Node '%s' not found in cluster, skipping", nodeName)
			}
		}
		return validNodes, nil
	}

	// Return all running nodes
	var runningNodes []string
	for nodeName := range allNodeIPs {
		node := l.cluster.GetNodeByName(nodeName)
		if node != nil && node.Phase == config.PhaseRunning {
			runningNodes = append(runningNodes, nodeName)
		}
	}

	return runningNodes, nil
}

// LoadImageToNodes loads a container image to specified nodes
func (l *ImageLoader) LoadImageToNodes(imageName string, nodes []string, skipArchCheck bool) error {
	// Step 1: Inspect image for architecture compatibility
	if !skipArchCheck {
		if err := l.validateImageArchitecture(imageName, nodes); err != nil {
			log.Warningf("Architecture validation warning: %v", err)
			log.Warningf("Use --skip-arch-check to bypass this validation")
		}
	}

	// Step 2: Export image from container runtime
	exportPath, err := l.exportImageFromRuntime(imageName)
	if err != nil {
		return fmt.Errorf("failed to export image from Docker: %w", err)
	}
	defer os.Remove(exportPath) // Clean up temporary file

	log.Infof("✅ Image exported to: %s", exportPath)

	// Step 3: Load image to each node
	var lastError error
	successCount := 0
	totalNodes := len(nodes)

	log.Infof("📤 Loading image to %d node(s)...", totalNodes)

	for i, nodeName := range nodes {
		log.Infof("📤 [%d/%d] Loading image to node '%s'...", i+1, totalNodes, nodeName)

		if err := l.loadImageToNode(exportPath, nodeName); err != nil {
			log.Errorf("❌ [%d/%d] Failed to load image to node '%s': %v", i+1, totalNodes, nodeName, err)
			lastError = err
		} else {
			log.Infof("✅ [%d/%d] Successfully loaded image to node '%s'", i+1, totalNodes, nodeName)
			successCount++
		}
	}

	if successCount == 0 {
		return fmt.Errorf("failed to load image to any nodes: %v", lastError)
	}

	if successCount < len(nodes) {
		log.Warningf("Image loaded to %d/%d nodes", successCount, len(nodes))
	} else {
		log.Infof("🎉 Successfully loaded image '%s' to all %d nodes", imageName, successCount)
	}

	return nil
}

// exportImageFromRuntime exports an image using the configured container runtime
func (l *ImageLoader) exportImageFromRuntime(imageName string) (string, error) {
	// Check if runtime is available
	if !l.containerRuntime.IsAvailable() {
		return "", fmt.Errorf("%s is not available", l.containerRuntime.Name())
	}

	// Create temporary file with safe name
	tmpDir := os.TempDir()
	safeName := l.makeSafeFileName(imageName)
	exportPath := filepath.Join(tmpDir, fmt.Sprintf("ohmykube-image-%s.tar", safeName))

	log.Infof("📦 Exporting image '%s' using %s...", imageName, l.containerRuntime.Name())

	// Export image using container runtime
	if err := l.containerRuntime.ExportImage(imageName, exportPath); err != nil {
		return "", fmt.Errorf("failed to export image '%s' using %s: %w", imageName, l.containerRuntime.Name(), err)
	}

	// Check if file was created and has content
	stat, err := os.Stat(exportPath)
	if err != nil {
		return "", fmt.Errorf("exported image file not found: %w", err)
	}
	if stat.Size() == 0 {
		return "", fmt.Errorf("exported image file is empty")
	}

	log.Infof("✅ Image exported successfully (size: %.2f MB)", float64(stat.Size())/(1024*1024))
	return exportPath, nil
}

// createContainerRuntime creates the appropriate container runtime
func CreateContainerRuntime(runtimeName string) (ContainerRuntimeInterface, error) {
	var runtime ContainerRuntimeInterface

	switch ContainerRuntime(runtimeName) {
	case DockerRuntimeType, "":
		runtime = &DockerRuntime{}
	case PodmanRuntimeType:
		runtime = &PodmanRuntime{}
	case NerdctlRuntimeType:
		runtime = &NerdctlRuntime{}
	default:
		return nil, fmt.Errorf("unsupported container runtime: %s. Supported: docker, podman, nerdctl", runtimeName)
	}

	// Check if runtime is available
	if !runtime.IsAvailable() {
		return nil, fmt.Errorf("%s is not available on this system", runtime.Name())
	}

	return runtime, nil
}

// DockerRuntime implements Docker operations
type DockerRuntime struct{}

func (d *DockerRuntime) Name() string { return "docker" }

func (d *DockerRuntime) IsAvailable() bool {
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	return cmd.Run() == nil
}

func (d *DockerRuntime) ExportImage(imageName, outputPath string) error {
	cmd := exec.Command("docker", "save", imageName, "-o", outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}
	return nil
}

func (d *DockerRuntime) InspectImage(imageName string) (*ImageInfo, error) {
	cmd := exec.Command("docker", "inspect", imageName, "--format", "{{.Architecture}},{{.Os}},{{.Size}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect image: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), ",")
	if len(parts) < 3 {
		return nil, fmt.Errorf("unexpected inspect output format")
	}

	size := int64(0)
	if s, err := fmt.Sscanf(parts[2], "%d", &size); err != nil || s != 1 {
		log.Warningf("Failed to parse image size: %v", err)
	}

	return &ImageInfo{
		Architecture: parts[0],
		Platform:     parts[1],
		Size:         size,
	}, nil
}

// PodmanRuntime implements Podman operations
type PodmanRuntime struct{}

func (p *PodmanRuntime) Name() string { return "podman" }

func (p *PodmanRuntime) IsAvailable() bool {
	cmd := exec.Command("podman", "version", "--format", "{{.Server.Version}}")
	return cmd.Run() == nil
}

func (p *PodmanRuntime) ExportImage(imageName, outputPath string) error {
	cmd := exec.Command("podman", "save", imageName, "-o", outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}
	return nil
}

func (p *PodmanRuntime) InspectImage(imageName string) (*ImageInfo, error) {
	cmd := exec.Command("podman", "inspect", imageName, "--format", "{{.Architecture}},{{.Os}},{{.Size}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect image: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), ",")
	if len(parts) < 3 {
		return nil, fmt.Errorf("unexpected inspect output format")
	}

	size := int64(0)
	if s, err := fmt.Sscanf(parts[2], "%d", &size); err != nil || s != 1 {
		log.Warningf("Failed to parse image size: %v", err)
	}

	return &ImageInfo{
		Architecture: parts[0],
		Platform:     parts[1],
		Size:         size,
	}, nil
}

// NerdctlRuntime implements nerdctl operations
type NerdctlRuntime struct{}

func (n *NerdctlRuntime) Name() string { return "nerdctl" }

func (n *NerdctlRuntime) IsAvailable() bool {
	cmd := exec.Command("nerdctl", "version")
	return cmd.Run() == nil
}

func (n *NerdctlRuntime) ExportImage(imageName, outputPath string) error {
	cmd := exec.Command("nerdctl", "save", imageName, "-o", outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}
	return nil
}

func (n *NerdctlRuntime) InspectImage(imageName string) (*ImageInfo, error) {
	cmd := exec.Command("nerdctl", "inspect", imageName, "--format", "{{.Architecture}},{{.Os}},{{.Size}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect image: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), ",")
	if len(parts) < 3 {
		return nil, fmt.Errorf("unexpected inspect output format")
	}

	size := int64(0)
	if s, err := fmt.Sscanf(parts[2], "%d", &size); err != nil || s != 1 {
		log.Warningf("Failed to parse image size: %v", err)
	}

	return &ImageInfo{
		Architecture: parts[0],
		Platform:     parts[1],
		Size:         size,
	}, nil
}

// makeSafeFileName converts an image name to a safe filename
func (l *ImageLoader) makeSafeFileName(imageName string) string {
	// Replace unsafe characters with underscores
	safe := strings.NewReplacer(
		":", "_",
		"/", "_",
		"@", "_",
		" ", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"\"", "_",
		"?", "_",
		"*", "_",
	).Replace(imageName)

	// Ensure filename is not too long
	if len(safe) > 100 {
		safe = safe[:100]
	}

	return safe
}

// loadImageToNode loads an image tar file to a specific node
func (l *ImageLoader) loadImageToNode(imagePath, nodeName string) error {
	nodeIP := l.cluster.GetNodeByName(nodeName).IP
	if nodeIP == "" {
		return fmt.Errorf("no IP found for node '%s'", nodeName)
	}

	// Create SSH client
	client := ssh.NewClient(nodeIP, "22", "root", l.sshConfig.Password, l.sshConfig.GetSSHKey())

	if err := client.Connect(); err != nil {
		return fmt.Errorf("failed to connect to node '%s' (%s): %w", nodeName, nodeIP, err)
	}
	defer client.Close()

	// Upload image file
	remoteImagePath := fmt.Sprintf("/tmp/%s", filepath.Base(imagePath))
	log.Debugf("Uploading image to %s:%s", nodeIP, remoteImagePath)

	if err := client.UploadFile(imagePath, remoteImagePath); err != nil {
		return fmt.Errorf("failed to upload image to node '%s': %w", nodeName, err)
	}

	// Load image using containerd
	loadCmd := fmt.Sprintf("ctr -n k8s.io images import %s", remoteImagePath)
	log.Debugf("Executing on %s: %s", nodeIP, loadCmd)

	if _, err := client.RunCommand(loadCmd); err != nil {
		return fmt.Errorf("failed to load image on node '%s': %w", nodeName, err)
	}

	// Clean up remote file
	cleanupCmd := fmt.Sprintf("rm -f %s", remoteImagePath)
	if _, err := client.RunCommand(cleanupCmd); err != nil {
		log.Warningf("Failed to cleanup remote image file on node '%s': %v", nodeName, err)
	}

	return nil
}

// validateImageArchitecture checks if image architecture matches node architectures
func (l *ImageLoader) validateImageArchitecture(imageName string, nodeNames []string) error {
	// Get image info
	imageInfo, err := l.containerRuntime.InspectImage(imageName)
	if err != nil {
		return fmt.Errorf("failed to inspect image: %w", err)
	}

	log.Infof("📊 Image info: architecture=%s, platform=%s", imageInfo.Architecture, imageInfo.Platform)

	// Check node architectures
	for _, nodeName := range nodeNames {
		nodeArch, err := l.getNodeArchitecture(nodeName)
		if err != nil {
			log.Warningf("Failed to get architecture for node %s: %v", nodeName, err)
			continue
		}

		if !l.isArchitectureCompatible(imageInfo.Architecture, nodeArch) {
			return fmt.Errorf("image architecture '%s' may not be compatible with node '%s' architecture '%s'",
				imageInfo.Architecture, nodeName, nodeArch)
		}
	}

	log.Infof("✅ Architecture validation passed")
	return nil
}

// getNodeArchitecture gets the CPU architecture of a node
func (l *ImageLoader) getNodeArchitecture(nodeName string) (string, error) {
	nodeIP := l.cluster.GetNodeByName(nodeName).IP
	if nodeIP == "" {
		return "", fmt.Errorf("no IP found for node '%s'", nodeName)
	}

	client := ssh.NewClient(nodeIP, "22", "root", l.sshConfig.Password, l.sshConfig.GetSSHKey())
	if err := client.Connect(); err != nil {
		return "", fmt.Errorf("failed to connect to node: %w", err)
	}
	defer client.Close()

	output, err := client.RunCommand("uname -m")
	if err != nil {
		return "", fmt.Errorf("failed to get architecture: %w", err)
	}

	return strings.TrimSpace(output), nil
}

// isArchitectureCompatible checks if image and node architectures are compatible
func (l *ImageLoader) isArchitectureCompatible(imageArch, nodeArch string) bool {
	// Normalize architectures
	imageArch = normalizeArchitecture(imageArch)
	nodeArch = normalizeArchitecture(nodeArch)

	// Direct match
	if imageArch == nodeArch {
		return true
	}

	// Common compatibility cases
	compatibilityMap := map[string][]string{
		"amd64": {"x86_64", "x86-64"},
		"arm64": {"aarch64", "arm64v8"},
		"386":   {"i386", "i686"},
	}

	if compatList, exists := compatibilityMap[imageArch]; exists {
		for _, compatible := range compatList {
			if compatible == nodeArch {
				return true
			}
		}
	}

	return false
}

// normalizeArchitecture converts architecture names to standard format
func normalizeArchitecture(arch string) string {
	switch strings.ToLower(arch) {
	case "x86_64", "x86-64", "amd64":
		return "amd64"
	case "aarch64", "arm64v8", "arm64":
		return "arm64"
	case "i386", "i686", "386":
		return "386"
	default:
		return strings.ToLower(arch)
	}
}
