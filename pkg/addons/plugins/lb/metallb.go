package lb

import (
	"context"
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/cache"
	"github.com/monshunter/ohmykube/pkg/config/default/metallb"
	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
)

// MetalLBInstaller used for installing and configuring MetalLB load balancer
type MetalLBInstaller struct {
	sshRunner      interfaces.SSHRunner
	controllerNode string
	controllerIP   string
	Version        string
	manifestURL    string
}

// NewMetalLBInstaller creates a new MetalLB installer
func NewMetalLBInstaller(sshRunner interfaces.SSHRunner, controllerNode string, controllerIP string) *MetalLBInstaller {
	installer := &MetalLBInstaller{
		sshRunner:      sshRunner,
		controllerNode: controllerNode,
		controllerIP:   controllerIP,
		Version:        "v0.14.9",
	}
	installer.manifestURL = fmt.Sprintf("https://raw.githubusercontent.com/metallb/metallb/%s/config/manifests/metallb-native.yaml", installer.Version)
	return installer
}

// Install installs MetalLB load balancer
func (m *MetalLBInstaller) Install() error {
	// Cache required images before installation
	if err := m.cacheImages(); err != nil {
		log.Warningf("Failed to cache MetalLB images: %v", err)
		// Continue with installation even if image caching fails
	}

	// Install MetalLB
	metallbCmd := `
kubectl apply -f %s
`
	metallbCmd = fmt.Sprintf(metallbCmd, m.manifestURL)
	_, err := m.sshRunner.RunCommand(m.controllerNode, metallbCmd)
	if err != nil {
		return fmt.Errorf("failed to install MetalLB: %w", err)
	}

	// Wait for MetalLB deployment to complete
	waitCmd := `
kubectl wait --namespace metallb-system --for=condition=ready pod --selector=app=metallb --timeout=90s
`
	_, err = m.sshRunner.RunCommand(m.controllerNode, waitCmd)
	if err != nil {
		return fmt.Errorf("failed to wait for MetalLB deployment to complete: %w", err)
	}

	// Get node IP address range
	ipRange, err := m.getMetalLBAddressRange()
	if err != nil {
		return fmt.Errorf("failed to get MetalLB address range: %w", err)
	}

	// Import configuration template
	configYAML := strings.Replace(metallb.CONFIG_YAML, "# - 192.168.64.200 - 192.168.64.250", fmt.Sprintf("- %s", ipRange), 1)

	// Create configuration file
	configFile := "/tmp/metallb-config.yaml"
	createConfigCmd := fmt.Sprintf("cat <<EOF | tee %s\n%sEOF", configFile, configYAML)
	_, err = m.sshRunner.RunCommand(m.controllerNode, createConfigCmd)
	if err != nil {
		return fmt.Errorf("failed to create MetalLB configuration file: %w", err)
	}

	// Apply configuration
	applyConfigCmd := fmt.Sprintf("kubectl apply -f %s", configFile)
	_, err = m.sshRunner.RunCommand(m.controllerNode, applyConfigCmd)
	if err != nil {
		return fmt.Errorf("failed to apply MetalLB configuration: %w", err)
	}

	return nil
}

// getMetalLBAddressRange gets suitable IP address range for MetalLB
func (m *MetalLBInstaller) getMetalLBAddressRange() (string, error) {
	// Parse IP address
	ipParts := strings.Split(m.controllerIP, ".")
	if len(ipParts) != 4 {
		return "", fmt.Errorf("invalid IP address format: %s", m.controllerIP)
	}

	// Use a range of IP addresses in the same subnet as LoadBalancer IP pool
	// Example: 192.168.64.100 -> 192.168.64.200-192.168.64.250
	prefix := strings.Join(ipParts[:3], ".")
	startIP := 200
	endIP := 250

	return fmt.Sprintf("%s.%d - %s.%d", prefix, startIP, prefix, endIP), nil
}

// cacheImages caches required MetalLB images before installation
func (m *MetalLBInstaller) cacheImages() error {
	ctx := context.Background()

	// Get singleton image manager
	imageManager, err := cache.GetImageManager()
	if err != nil {
		return fmt.Errorf("failed to get image manager: %w", err)
	}

	// Define MetalLB manifest source
	source := cache.ImageSource{
		Type:        "manifest",
		ManifestURL: m.manifestURL,
		Version:     m.Version,
	}

	// Cache images for all nodes in the cluster
	// For now, we'll cache for the controller node
	err = imageManager.EnsureImages(ctx, source, m.sshRunner, m.controllerNode, m.controllerNode)
	if err != nil {
		return fmt.Errorf("failed to cache MetalLB images: %w", err)
	}
	_ = imageManager.ReCacheClusterImages(m.sshRunner)
	return nil
}
