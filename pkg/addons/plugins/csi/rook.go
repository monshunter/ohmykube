package csi

import (
	"context"
	"fmt"

	"github.com/monshunter/ohmykube/pkg/cache"
	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
)

// RookInstaller is responsible for installing Rook-Ceph CSI
type RookInstaller struct {
	sshRunner      interfaces.SSHRunner
	controllerNode string
	RookVersion    string
	CephVersion    string
}

// NewRookInstaller creates a Rook installer
func NewRookInstaller(sshRunner interfaces.SSHRunner, controllerNode string) *RookInstaller {
	return &RookInstaller{
		sshRunner:      sshRunner,
		controllerNode: controllerNode,
		RookVersion:    "v1.17.2", // Rook version
		CephVersion:    "17.2.6",  // Ceph version
	}
}

// Install installs Rook-Ceph
func (r *RookInstaller) Install() error {
	// Cache required images before installation
	if err := r.cacheImages(); err != nil {
		log.Warningf("Failed to cache Rook-Ceph images: %v", err)
		// Continue with installation even if image caching fails
	}

	// Use Helm to install Rook-Ceph
	log.Infof("Installing Rook-Ceph with Rook version %s and Ceph version %s...", r.RookVersion, r.CephVersion)
	helmInstallCmd := `
# Add Rook Helm repository
helm repo add rook-release https://charts.rook.io/release
helm repo update

# Install Rook Ceph Operator
helm install --create-namespace --namespace rook-ceph rook-ceph rook-release/rook-ceph

# Wait for operator pod to be ready
kubectl wait --for=condition=ready pod -l app=rook-ceph-operator -n rook-ceph --timeout=300s

# Install Rook Ceph Cluster
helm install --create-namespace --namespace rook-ceph rook-ceph-cluster \
   --set operatorNamespace=rook-ceph rook-release/rook-ceph-cluster
`

	_, err := r.sshRunner.RunCommand(r.controllerNode, helmInstallCmd)
	if err != nil {
		return fmt.Errorf("failed to install Rook-Ceph CSI: %w", err)
	}

	log.Info("Rook-Ceph installation completed successfully")
	return nil
}

// cacheImages caches required Rook-Ceph images before installation
func (r *RookInstaller) cacheImages() error {
	ctx := context.Background()

	// Create image manager with default configuration
	imageManager, err := cache.NewImageManager()
	if err != nil {
		return fmt.Errorf("failed to create image manager: %w", err)
	}

	// Define Rook Helm chart sources
	// Cache images for both rook-ceph operator and cluster
	sources := []cache.ImageSource{
		{
			Type:      "helm",
			ChartName: "rook-release/rook-ceph",
			ChartRepo: "https://charts.rook.io/release",
			Version:   r.RookVersion, // Use latest version
		},
		{
			Type:      "helm",
			ChartName: "rook-release/rook-ceph-cluster",
			ChartRepo: "https://charts.rook.io/release",
			Version:   r.CephVersion, // Use latest version
			ChartValues: map[string]string{
				"operatorNamespace": "rook-ceph",
			},
		},
	}

	// Cache images for each source
	for _, source := range sources {
		if err := imageManager.EnsureImages(ctx, source, r.sshRunner, r.controllerNode, r.controllerNode); err != nil {
			log.Warningf("Failed to cache images for %s: %v", source.ChartName, err)
			// Continue with other sources
		}
	}
	_ = imageManager.ReCacheClusterImages(r.sshRunner)
	return nil
}
