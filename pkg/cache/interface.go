package cache

import (
	"context"
	"time"

	"github.com/monshunter/ohmykube/pkg/interfaces"
)

// SSHRunner defines the interface for SSH operations (commands + file transfer)
type SSHRunner = interfaces.SSHRunner

// SSHCommandRunner defines the interface for executing SSH commands (for backward compatibility)
type SSHCommandRunner = interfaces.SSHCommandRunner

// PackageCacheManager defines the interface for package cache management
type PackageCacheManager interface {
	// EnsurePackage ensures the specified package is available both locally and on target nodes (legacy)
	EnsurePackage(ctx context.Context, packageName, version, arch string, sshRunner SSHCommandRunner, nodeName string) error

	// EnsurePackageWithSCP ensures the specified package is available both locally and on target nodes using SCP
	EnsurePackageWithSCP(ctx context.Context, packageName, version, arch string, sshRunner SSHRunner, nodeName string) error

	// GetLocalPackagePath returns the local path of a cached package
	GetLocalPackagePath(packageName, version, arch string) (string, error)

	// IsPackageCached checks if a package is already cached locally
	IsPackageCached(packageName, version, arch string) bool

	// UploadPackageToNode uploads a cached package to a target node (legacy)
	UploadPackageToNode(ctx context.Context, packageName, version, arch string, sshRunner SSHCommandRunner, nodeName string) error

	// UploadPackageWithSCP uploads a cached package to a target node using SCP
	UploadPackageWithSCP(ctx context.Context, packageName, version, arch string, sshRunner SSHRunner, nodeName string) error
}

// ImageCacheManager defines the interface for image cache management
type ImageCacheManager interface {
	// EnsureImages ensures all required images are cached and available on the target node
	EnsureImages(ctx context.Context, source ImageSource, sshRunner SSHRunner, nodeName string, controllerNode string) error

	// EnsureImage ensures a specific image is cached and available on the target node
	EnsureImage(ctx context.Context, image string, sshRunner SSHRunner, nodeName string, controllerNode string) error

	// IsImageCached checks if an image is already cached locally
	IsImageCached(cacheKey string) bool

	// GetLocalImagePath returns the local path of a cached image
	GetLocalImagePath(cacheKey string) (string, error)

	// CleanupOldImages removes images older than the specified duration
	CleanupOldImages(maxAge time.Duration) error

	// GetCacheStats returns statistics about the image cache
	GetCacheStats() (int, int64)
}
