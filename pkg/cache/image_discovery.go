package cache

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/utils"
)

// DefaultImageDiscovery implements the ImageDiscovery interface
type DefaultImageDiscovery struct{}

// NewDefaultImageDiscovery creates a new image discovery instance (for controller/remote operations)
func NewDefaultImageDiscovery() *DefaultImageDiscovery {
	return &DefaultImageDiscovery{}
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

// getHelmChartImages extracts images from a Helm chart without installing it (runs on controller node)
func (d *DefaultImageDiscovery) getHelmChartImages(ctx context.Context, source ImageSource, sshRunner SSHRunner, controllerNode string) ([]string, error) {
	log.Debugf("Discovering Helm chart images on controller node: %s", controllerNode)

	// Handle values files - upload them to controller node if they exist locally
	remoteValuesFiles, err := d.prepareValuesFiles(ctx, source.ValuesFile, sshRunner, controllerNode)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare values files: %w", err)
	}

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

# Build values file arguments
VALUES_FILE_ARGS=""
%s

# Template the chart
log_with_time "Templating chart: $CHART_NAME with args: $VALUES_ARGS $VALUES_FILE_ARGS"
helm template %s %s $VALUES_ARGS $VALUES_FILE_ARGS 2>&1
`

	// Build values arguments
	var valuesBuilder strings.Builder
	for k, v := range source.ChartValues {
		valuesBuilder.WriteString(fmt.Sprintf("VALUES_ARGS=\"$VALUES_ARGS --set %s=%s\"\n", k, v))
	}

	// Build values file arguments using remote paths
	var valuesFileBuilder strings.Builder
	for _, remoteValuesFile := range remoteValuesFiles {
		valuesFileBuilder.WriteString(fmt.Sprintf("VALUES_FILE_ARGS=\"$VALUES_FILE_ARGS --values %s\"\n", remoteValuesFile))
	}

	// Build version argument
	versionArg := ""
	if source.Version != "" {
		versionArg = fmt.Sprintf("--version %s", source.Version)
	}

	// Format the script
	formattedScript := fmt.Sprintf(script, source.ChartRepo, source.ChartName, valuesBuilder.String(), valuesFileBuilder.String(), source.ChartName, versionArg)

	// Execute on controller node
	output, err := sshRunner.RunCommand(controllerNode, formattedScript)
	if err != nil {
		return nil, fmt.Errorf("failed to template helm chart on controller node: %w", err)
	}

	// Extract images from the templated output
	return utils.ExtractImagesWithParser(output)
}

// prepareValuesFiles handles values files by uploading local files to controller node or downloading remote files
func (d *DefaultImageDiscovery) prepareValuesFiles(ctx context.Context, valuesFiles []string, sshRunner SSHRunner, controllerNode string) ([]string, error) {
	valuesFileManager := NewDefaultValuesFileManager()
	return valuesFileManager.PrepareValuesFiles(ctx, valuesFiles, sshRunner, controllerNode, "helm-discovery")
}


// getKubeadmImages gets required images for kubeadm (runs on controller node)
func (d *DefaultImageDiscovery) getKubeadmImages(ctx context.Context, source ImageSource, sshRunner SSHRunner, controllerNode string) ([]string, error) {
	log.Debugf("Discovering kubeadm images on controller node: %s", controllerNode)

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
	log.Debugf("Discovering manifest images on controller node: %s", controllerNode)

	var allImages []string
	
	// Process each manifest file
	for _, manifestFile := range source.ManifestFiles {
		var output string
		var err error
		
		// Check if it's a URL
		if strings.HasPrefix(manifestFile, "http://") || strings.HasPrefix(manifestFile, "https://") {
			// Download the manifest on controller node
			cmd := fmt.Sprintf("curl -s -L %s", manifestFile)
			output, err = sshRunner.RunCommand(controllerNode, cmd)
			if err != nil {
				return nil, fmt.Errorf("failed to download manifest from %s on controller node: %w", manifestFile, err)
			}
		} else {
			// Assume it's a local file path - read it directly from local machine
			content, err := os.ReadFile(manifestFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read local manifest file %s: %w", manifestFile, err)
			}
			output = string(content)
		}
		
		// Extract images from this manifest
		images, err := utils.ExtractImagesWithParser(output)
		if err != nil {
			return nil, fmt.Errorf("failed to extract images from manifest %s: %w", manifestFile, err)
		}
		
		allImages = append(allImages, images...)
	}
	
	// Remove duplicates
	seen := make(map[string]bool)
	var uniqueImages []string
	for _, image := range allImages {
		if !seen[image] {
			seen[image] = true
			uniqueImages = append(uniqueImages, image)
		}
	}
	
	return uniqueImages, nil
}

// DefaultValuesFileManager is a concrete implementation of ValuesFileManager
type DefaultValuesFileManager struct{}

// NewDefaultValuesFileManager creates a new instance of DefaultValuesFileManager
func NewDefaultValuesFileManager() interfaces.ValuesFileManager {
	return &DefaultValuesFileManager{}
}

// PrepareValuesFiles handles values files by uploading local files or downloading remote files
func (m *DefaultValuesFileManager) PrepareValuesFiles(ctx context.Context, valuesFiles []string, sshRunner interfaces.SSHRunner, controllerNode string, prefix string) ([]string, error) {
	if len(valuesFiles) == 0 {
		return nil, nil
	}

	remoteValuesFiles := make([]string, 0, len(valuesFiles))

	for i, valuesFile := range valuesFiles {
		remotePath, err := m.prepareValuesFile(ctx, valuesFile, sshRunner, controllerNode, prefix, i)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare values file %s: %w", valuesFile, err)
		}
		remoteValuesFiles = append(remoteValuesFiles, remotePath)
	}

	return remoteValuesFiles, nil
}

// prepareValuesFile handles a single values file
func (m *DefaultValuesFileManager) prepareValuesFile(ctx context.Context, valuesFile string, sshRunner interfaces.SSHRunner, controllerNode string, prefix string, index int) (string, error) {
	// Check if it's a URL
	if strings.HasPrefix(valuesFile, "http://") || strings.HasPrefix(valuesFile, "https://") {
		// Download the file on the controller node
		remotePath := fmt.Sprintf("/tmp/%s-values-%d.yaml", prefix, index)
		downloadCmd := fmt.Sprintf("curl -s -L -o %s %s", remotePath, valuesFile)

		log.Debugf("Downloading values file from %s to %s on controller node", valuesFile, remotePath)
		_, err := sshRunner.RunCommand(controllerNode, downloadCmd)
		if err != nil {
			return "", fmt.Errorf("failed to download values file from %s: %w", valuesFile, err)
		}

		// Verify the downloaded file is not empty
		checkCmd := fmt.Sprintf("test -s %s", remotePath)
		_, err = sshRunner.RunCommand(controllerNode, checkCmd)
		if err != nil {
			return "", fmt.Errorf("downloaded file %s is empty or does not exist", valuesFile)
		}

		return remotePath, nil
	}

	// Check if it's a local file
	if _, err := os.Stat(valuesFile); err == nil {
		// Upload the local file to controller node
		remotePath := fmt.Sprintf("/tmp/%s-values-%d.yaml", prefix, index)

		log.Debugf("Uploading local values file from %s to %s on controller node", valuesFile, remotePath)
		err := sshRunner.UploadFile(controllerNode, valuesFile, remotePath)
		if err != nil {
			return "", fmt.Errorf("failed to upload values file %s: %w", valuesFile, err)
		}

		return remotePath, nil
	}

	// If it's neither a URL nor a local file, assume it's already a remote path
	log.Debugf("Using values file path as-is (assuming it exists on controller node): %s", valuesFile)
	return valuesFile, nil
}
