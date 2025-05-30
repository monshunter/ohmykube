package interfaces

import (
	"context"
)

// PackageCacheManager defines the interface for package cache management
type PackageCacheManager interface {
	// EnsurePackage ensures the specified package is available both locally and on target nodes (legacy)
	EnsurePackage(ctx context.Context, packageName, version, arch string, sshRunner SSHRunner, nodeName string) error

	// IsPackageCached checks if a package is already cached locally
	IsPackageCached(packageName, version, arch string) bool
}

// ImageCacheManager defines the interface for image cache management
type ImageCacheManager interface {
	// EnsureImages ensures all required images are cached and available on the target node
	EnsureImages(ctx context.Context, source ImageSource, sshRunner SSHRunner, nodeName string, controllerNode string) error

	// EnsureImage ensures a specific image is cached and available on the target node
	EnsureImage(ctx context.Context, image string, sshRunner SSHRunner, nodeName string, controllerNode string) error

	// IsImageCached checks if an image is already cached locally
	IsImageCached(cacheKey string) bool
}

// ImageDiscovery provides methods to discover required images
type ImageDiscovery interface {
	// GetRequiredImages returns a list of images required for a specific application/version
	GetRequiredImages(ctx context.Context, source ImageSource, sshRunner SSHRunner, controllerNode string) ([]string, error)
}

// ImageSource defines where and how to discover required images
type ImageSource struct {
	Type        string            // "helm", "manifest", "kubeadm", "custom"
	ChartName   string            // For helm charts
	ChartRepo   string            // For helm charts
	ChartValues map[string]string // For helm charts
	ManifestURL string            // For kubernetes manifests
	Version     string            // Version information
}
