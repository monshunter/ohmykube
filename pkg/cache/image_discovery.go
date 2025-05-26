package cache

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/utils"
)

// DefaultImageDiscovery implements the ImageDiscovery interface
type DefaultImageDiscovery struct{}

// NewDefaultImageDiscovery creates a new image discovery instance (for controller/remote operations)
func NewDefaultImageDiscovery() *DefaultImageDiscovery {
	return &DefaultImageDiscovery{}
}

// LocalImageDiscovery implements image discovery using local tools
type LocalImageDiscovery struct{}

// NewLocalImageDiscovery creates a new local image discovery instance
func NewLocalImageDiscovery() *LocalImageDiscovery {
	return &LocalImageDiscovery{}
}

// GetRequiredImagesForArch discovers required images for a specific architecture using local tools
func (d *LocalImageDiscovery) GetRequiredImagesForArch(ctx context.Context, source ImageSource, arch string) ([]string, error) {
	// Get basic image list using local tools
	images, err := d.getRequiredImagesLocally(ctx, source)
	if err != nil {
		return nil, err
	}

	// All images are processed uniformly - no special handling needed
	// The image reference parsing and caching system handles all formats robustly
	return images, nil
}

// getRequiredImagesLocally discovers required images using local tools
func (d *LocalImageDiscovery) getRequiredImagesLocally(ctx context.Context, source ImageSource) ([]string, error) {
	switch source.Type {
	case "helm":
		return d.getHelmChartImagesLocally(ctx, source)
	case "manifest":
		return d.getManifestImagesLocally(ctx, source)
	case "kubeadm":
		return d.getKubeadmImagesLocally(ctx, source)
	default:
		return nil, fmt.Errorf("unsupported image source type: %s", source.Type)
	}
}

// GetRequiredImages discovers required images based on the source type
func (d *DefaultImageDiscovery) GetRequiredImages(ctx context.Context, source ImageSource, sshRunner SSHRunner, controllerNode string) ([]string, error) {
	switch source.Type {
	case "helm":
		return d.getHelmChartImages(ctx, source, sshRunner, controllerNode)
	case "manifest":
		return d.getManifestImages(ctx, source, sshRunner, controllerNode)
	case "kubeadm":
		return d.getKubeadmImages(ctx, source, sshRunner, controllerNode)
	default:
		return nil, fmt.Errorf("unsupported image source type: %s", source.Type)
	}
}

// GetRequiredImagesForArch discovers required images for a specific architecture
func (d *DefaultImageDiscovery) GetRequiredImagesForArch(ctx context.Context, source ImageSource, arch string, sshRunner SSHRunner, controllerNode string) ([]string, error) {
	// Get basic image list
	images, err := d.GetRequiredImages(ctx, source, sshRunner, controllerNode)
	if err != nil {
		return nil, err
	}

	// All images are processed uniformly - no special handling needed
	// The image reference parsing and caching system handles all formats robustly
	return images, nil
}

// getHelmChartImages extracts images from a Helm chart without installing it (runs on controller node)
func (d *DefaultImageDiscovery) getHelmChartImages(ctx context.Context, source ImageSource, sshRunner SSHRunner, controllerNode string) ([]string, error) {
	log.Infof("Discovering Helm chart images on controller node: %s", controllerNode)

	// Build the helm command script to run on controller node
	script := `#!/bin/bash
set -e

# Function to log with timestamp
log_with_time() {
    echo "[$(date '+%%Y-%%m-%%d %%H:%%M:%%S')] $1"
}

# Add repo if specified
CHART_REPO="%s"
CHART_NAME="%s"
if [ -n "$CHART_REPO" ]; then
    # Extract repository name from chart name (e.g., "flannel/flannel" -> "flannel")
    REPO_NAME=$(echo "$CHART_NAME" | cut -d'/' -f1)
    log_with_time "Adding Helm repository: $REPO_NAME -> $CHART_REPO"
    helm repo add "$REPO_NAME" "$CHART_REPO" || true
fi

# Update repos
log_with_time "Updating Helm repositories..."
helm repo update

# Build values arguments
VALUES_ARGS=""
%s

# Template the chart
log_with_time "Templating chart: $CHART_NAME with args: $VALUES_ARGS"
helm template %s %s $VALUES_ARGS 2>&1
`

	// Build values arguments
	var valuesBuilder strings.Builder
	for k, v := range source.ChartValues {
		valuesBuilder.WriteString(fmt.Sprintf("VALUES_ARGS=\"$VALUES_ARGS --set %s=%s\"\n", k, v))
	}

	// Build version argument
	versionArg := ""
	if source.Version != "" {
		versionArg = fmt.Sprintf("--version %s", source.Version)
	}

	// Format the script
	formattedScript := fmt.Sprintf(script, source.ChartRepo, source.ChartName, valuesBuilder.String(), source.ChartName, versionArg)

	// Execute on controller node
	output, err := sshRunner.RunCommand(controllerNode, formattedScript)
	if err != nil {
		return nil, fmt.Errorf("failed to template helm chart on controller node: %w", err)
	}

	// Extract images from the templated output
	return utils.ExtractImagesWithParser(output)
}

// getKubeadmImages gets required images for kubeadm (runs on controller node)
func (d *DefaultImageDiscovery) getKubeadmImages(ctx context.Context, source ImageSource, sshRunner SSHRunner, controllerNode string) ([]string, error) {
	log.Infof("Discovering kubeadm images on controller node: %s", controllerNode)

	// Run kubeadm command on controller node
	cmd := fmt.Sprintf("kubeadm config images list --kubernetes-version %s", source.Version)
	output, err := sshRunner.RunCommand(controllerNode, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeadm images on controller node: %w", err)
	}

	images := []string{}
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			images = append(images, line)
		}
	}
	return images, nil
}

// getManifestImages extracts images from kubernetes manifests (runs on controller node)
func (d *DefaultImageDiscovery) getManifestImages(ctx context.Context, source ImageSource, sshRunner SSHRunner, controllerNode string) ([]string, error) {
	log.Infof("Discovering manifest images on controller node: %s", controllerNode)

	// Download the manifest on controller node
	cmd := fmt.Sprintf("curl -s %s", source.ManifestURL)
	output, err := sshRunner.RunCommand(controllerNode, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to download manifest on controller node: %w", err)
	}

	// Extract images from the manifest
	return utils.ExtractImagesWithParser(output)
}

// ===== LOCAL IMAGE DISCOVERY METHODS =====
// getHelmChartImagesLocally extracts images from a Helm chart using local helm
func (d *LocalImageDiscovery) getHelmChartImagesLocally(ctx context.Context, source ImageSource) ([]string, error) {
	log.Infof("Discovering Helm chart images locally")

	// Add repo if specified
	if source.ChartRepo != "" {
		// Extract repository name from chart name (e.g., "flannel/flannel" -> "flannel")
		repoName := strings.Split(source.ChartName, "/")[0]
		log.Infof("Adding Helm repository locally: %s -> %s", repoName, source.ChartRepo)
		addRepoCmd := exec.CommandContext(ctx, "helm", "repo", "add", repoName, source.ChartRepo)
		if err := addRepoCmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to add helm repo locally: %w", err)
		}
	}

	// Update repos
	updateCmd := exec.CommandContext(ctx, "helm", "repo", "update")
	if err := updateCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to update helm repos locally: %w", err)
	}

	// Build values arguments
	var valuesArgs []string
	for k, v := range source.ChartValues {
		valuesArgs = append(valuesArgs, "--set", fmt.Sprintf("%s=%s", k, v))
	}

	// Template the chart to see what would be installed
	args := []string{"template", source.ChartName}
	if source.Version != "" {
		args = append(args, "--version", source.Version)
	}
	args = append(args, valuesArgs...)

	templateCmd := exec.CommandContext(ctx, "helm", args...)
	output, err := templateCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to template helm chart locally: %w", err)
	}
	// Extract images from the templated output
	return utils.ExtractImagesWithParser(string(output))
}

// getKubeadmImagesLocally gets required images for kubeadm using local kubeadm
func (d *LocalImageDiscovery) getKubeadmImagesLocally(ctx context.Context, source ImageSource) ([]string, error) {
	log.Infof("Discovering kubeadm images locally")

	cmd := exec.CommandContext(ctx, "kubeadm", "config", "images", "list", "--kubernetes-version", source.Version)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeadm images locally: %w", err)
	}

	images := []string{}
	for _, line := range strings.Split(string(output), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			images = append(images, line)
		}
	}
	return images, nil
}

// getManifestImagesLocally extracts images from kubernetes manifests using local curl
func (d *LocalImageDiscovery) getManifestImagesLocally(ctx context.Context, source ImageSource) ([]string, error) {
	log.Infof("Discovering manifest images locally")

	// Download the manifest using local curl
	curlCmd := exec.CommandContext(ctx, "curl", "-s", source.ManifestURL)
	output, err := curlCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to download manifest locally: %w", err)
	}

	// Extract images from the manifest
	return utils.ExtractImagesWithParser(string(output))
}
