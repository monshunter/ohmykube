package cni

import (
	"context"
	"fmt"

	"github.com/monshunter/ohmykube/pkg/cache"
	"github.com/monshunter/ohmykube/pkg/clusterinfo"
	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
)

// FlannelInstaller is responsible for installing Flannel CNI
type FlannelInstaller struct {
	sshRunner      interfaces.SSHRunner
	controllerNode string
	PodCIDR        string
	Version        string
}

// NewFlannelInstaller creates a Flannel installer
func NewFlannelInstaller(sshRunner interfaces.SSHRunner, controllerNode string) *FlannelInstaller {
	return &FlannelInstaller{
		sshRunner:      sshRunner,
		controllerNode: controllerNode,
		PodCIDR:        "10.244.0.0/16", // Flannel default Pod CIDR
		Version:        "v0.26.7",       // Flannel version
	}
}

// Install installs Flannel CNI using Helm
func (f *FlannelInstaller) Install() error {
	// Cache required images before installation
	if err := f.cacheImages(); err != nil {
		log.Warningf("Failed to cache Flannel images: %v", err)
		// Continue with installation even if image caching fails
	}

	// Ensure br_netfilter module is loaded
	brNetfilterCmd := `
sudo modprobe br_netfilter
echo "br_netfilter" | sudo tee /etc/modules-load.d/br_netfilter.conf
`
	_, err := f.sshRunner.RunCommand(f.controllerNode, brNetfilterCmd)
	if err != nil {
		return fmt.Errorf("failed to load br_netfilter module: %w", err)
	}

	// Use ClusterInfo to get the cluster's Pod CIDR
	clusterInfo := clusterinfo.NewClusterInfo(f.sshRunner, f.controllerNode)
	podCIDR, err := clusterInfo.GetPodCIDR()
	if err == nil && podCIDR != "" {
		f.PodCIDR = podCIDR
		log.Infof("Retrieved cluster Pod CIDR: %s", f.PodCIDR)
	}

	// Add official Flannel Helm repository
	log.Info("Adding official Flannel Helm repository...")
	addRepoCmd := `
helm repo add flannel https://flannel-io.github.io/flannel/
helm repo update
`
	_, err = f.sshRunner.RunCommand(f.controllerNode, addRepoCmd)
	if err != nil {
		return fmt.Errorf("failed to add Flannel Helm repository: %w", err)
	}

	// Install Flannel using Helm with minimal configuration
	log.Info("Installing Flannel using Helm...")
	installCmd := fmt.Sprintf(`
# Needs manual creation of namespace to avoid helm error
kubectl create ns kube-flannel
kubectl label --overwrite ns kube-flannel pod-security.kubernetes.io/enforce=privileged
helm install flannel flannel/flannel \
  --version %s \
  --set podCidr="%s" \
  --namespace kube-flannel \
  --create-namespace \
  --wait \
  --timeout 300s
`, f.Version, f.PodCIDR)

	_, err = f.sshRunner.RunCommand(f.controllerNode, installCmd)
	if err != nil {
		return fmt.Errorf("failed to install Flannel using Helm: %w", err)
	}

	// Wait for Flannel to be ready
	log.Info("Waiting for Flannel to become ready...")
	waitCmd := `kubectl -n kube-flannel wait --for=condition=ready pod -l app=flannel --timeout=300s`
	_, err = f.sshRunner.RunCommand(f.controllerNode, waitCmd)
	if err != nil {
		log.Warningf("Timed out waiting for Flannel to be ready, but will continue: %v", err)
	}

	log.Info("Flannel installation completed successfully")
	return nil
}

// SetPodCIDR sets the Pod CIDR
func (f *FlannelInstaller) SetPodCIDR(cidr string) {
	f.PodCIDR = cidr
}

// SetVersion sets the Flannel version
func (f *FlannelInstaller) SetVersion(version string) {
	f.Version = version
}

// cacheImages caches required Flannel images before installation
func (f *FlannelInstaller) cacheImages() error {
	ctx := context.Background()

	// First, ensure the Helm repository is added with the correct name
	log.Info("Adding Flannel Helm repository for image caching...")
	addRepoCmd := `
helm repo add flannel https://flannel-io.github.io/flannel/ || true
helm repo update
`
	_, err := f.sshRunner.RunCommand(f.controllerNode, addRepoCmd)
	if err != nil {
		return fmt.Errorf("failed to add Flannel Helm repository for caching: %w", err)
	}

	// Get singleton image manager
	imageManager, err := cache.GetImageManager()
	if err != nil {
		return fmt.Errorf("failed to get image manager: %w", err)
	}

	// Define Flannel Helm chart source (repository already added above)
	source := cache.ImageSource{
		Type:      "helm",
		ChartName: "flannel/flannel",
		ChartRepo: "", // Empty since repository is already added
		Version:   f.Version,
		ChartValues: map[string]string{
			"podCidr": f.PodCIDR,
		},
	}

	// Cache images for all nodes in the cluster
	// For now, we'll cache for the controller node
	err = imageManager.EnsureImages(ctx, source, f.sshRunner, f.controllerNode, f.controllerNode)
	if err != nil {
		return fmt.Errorf("failed to cache Flannel images: %w", err)
	}
	_ = imageManager.ReCacheClusterImages(f.sshRunner)
	return nil
}
