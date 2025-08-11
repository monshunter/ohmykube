package csi

import (
	"context"
	"fmt"

	"github.com/monshunter/ohmykube/pkg/cache"
	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
)

// LocalPathInstaller is responsible for installing local-path-provisioner CSI
type LocalPathInstaller struct {
	sshRunner      interfaces.SSHRunner
	controllerNode string
	manifestURL    string
	Version        string
}

// NewLocalPathInstaller creates a local-path-provisioner installer
func NewLocalPathInstaller(sshRunner interfaces.SSHRunner, controllerNode string) *LocalPathInstaller {
	installer := &LocalPathInstaller{
		sshRunner:      sshRunner,
		controllerNode: controllerNode,
		Version:        "v0.0.31", // local-path-provisioner version
	}
	installer.manifestURL = fmt.Sprintf("https://raw.githubusercontent.com/rancher/local-path-provisioner/%s/deploy/local-path-storage.yaml", installer.Version)
	return installer
}

// Install installs local-path-provisioner
func (l *LocalPathInstaller) Install() error {
	// Cache required images before installation
	if err := l.cacheImages(); err != nil {
		log.Warningf("Failed to cache local-path-provisioner images: %v", err)
		// Continue with installation even if image caching fails
	}

	// Install local-path-provisioner
	log.Debugf("Installing local-path-provisioner version %s...", l.Version)
	localPathCmd := fmt.Sprintf("kubectl apply -f %s", l.manifestURL)

	_, err := l.sshRunner.RunCommand(l.controllerNode, localPathCmd)
	if err != nil {
		return fmt.Errorf("failed to install local-path-provisioner: %w", err)
	}

	// Set local-path as default storage class
	log.Debug("Setting local-path as default storage class...")
	defaultStorageClassCmd := `
kubectl patch storageclass local-path -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
`
	_, err = l.sshRunner.RunCommand(l.controllerNode, defaultStorageClassCmd)
	if err != nil {
		return fmt.Errorf("failed to set local-path as default storage class: %w", err)
	}

	log.Debug("Local-path-provisioner installation completed successfully")
	return nil
}

// cacheImages caches required local-path-provisioner images before installation
func (l *LocalPathInstaller) cacheImages() error {
	ctx := context.Background()

	// Get singleton image manager
	imageManager, err := cache.GetImageManager()
	if err != nil {
		return fmt.Errorf("failed to get image manager: %w", err)
	}

	// Define local-path-provisioner manifest source
	source := cache.ImageSource{
		Type:         "manifest",
		ManifestFiles: []string{l.manifestURL},
		Version:      l.Version,
	}

	// Cache images for all nodes in the cluster
	// For now, we'll cache for the controller node
	err = imageManager.EnsureImages(ctx, source, l.sshRunner, l.controllerNode, l.controllerNode)
	if err != nil {
		return fmt.Errorf("failed to cache local-path-provisioner images: %w", err)
	}
	_ = imageManager.ReCacheClusterImages(l.sshRunner)
	return nil
}
