package cache

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/monshunter/ohmykube/pkg/envar"
	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
	"gopkg.in/yaml.v3"
)

// ImageManager manages container image caching
type ImageManager struct {
	cacheDir       string
	indexPath      string
	imageIndex     *ImageIndex
	imageDiscovery ImageDiscovery
	toolDetector   *ToolDetector
	config         ImageManagementConfig
	availability   *ToolAvailability
}

// NewImageManager creates a new image cache manager with default configuration
func NewImageManager() (*ImageManager, error) {
	return NewImageManagerWithConfig(DefaultImageManagementConfig())
}

// NewImageManagerWithConfig creates a new image cache manager with custom configuration
func NewImageManagerWithConfig(config ImageManagementConfig) (*ImageManager, error) {
	// Get cache directory from environment
	cacheDir := filepath.Join(envar.OhMyKubeCacheDir(), "images")
	indexPath := filepath.Join(cacheDir, "index.yaml")

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create image cache directory: %w", err)
	}

	manager := &ImageManager{
		cacheDir:       cacheDir,
		indexPath:      indexPath,
		imageDiscovery: NewDefaultImageDiscovery(),
		toolDetector:   NewToolDetector(),
		config:         config,
	}

	// Load existing index or create new one
	if err := manager.loadIndex(); err != nil {
		return nil, fmt.Errorf("failed to load image index: %w", err)
	}

	return manager, nil
}

// loadIndex loads the image index from disk or creates a new one
func (m *ImageManager) loadIndex() error {
	if _, err := os.Stat(m.indexPath); os.IsNotExist(err) {
		// Create new index
		m.imageIndex = &ImageIndex{
			Version:   "1.0",
			Images:    make(map[string]ImageInfo),
			UpdatedAt: time.Now(),
		}
		return m.saveIndex()
	}

	// Load existing index
	data, err := os.ReadFile(m.indexPath)
	if err != nil {
		return fmt.Errorf("failed to read index file: %w", err)
	}

	m.imageIndex = &ImageIndex{}
	if err := yaml.Unmarshal(data, m.imageIndex); err != nil {
		return fmt.Errorf("failed to parse index file: %w", err)
	}

	// Initialize images map if nil
	if m.imageIndex.Images == nil {
		m.imageIndex.Images = make(map[string]ImageInfo)
	}

	return nil
}

// saveIndex saves the image index to disk
func (m *ImageManager) saveIndex() error {
	m.imageIndex.UpdatedAt = time.Now()

	data, err := yaml.Marshal(m.imageIndex)
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	if err := os.WriteFile(m.indexPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}

	return nil
}

// EnsureImages ensures all required images are cached and available on the target node
func (m *ImageManager) EnsureImages(ctx context.Context, source ImageSource, sshRunner interfaces.SSHRunner, nodeName string, controllerNode string) error {
	// Initialize tool availability if not already done
	if m.availability == nil {
		if err := m.initializeToolAvailability(ctx, sshRunner, controllerNode); err != nil {
			return fmt.Errorf("failed to initialize tool availability: %w", err)
		}
	}

	// Get node architecture first
	arch, err := m.getNodeArchitecture(ctx, sshRunner, nodeName)
	if err != nil {
		return fmt.Errorf("failed to determine node architecture: %w", err)
	}

	// Determine optimal strategy
	strategy := m.toolDetector.DetermineOptimalStrategy(m.availability, m.config)
	log.Infof("Using %s strategy for image management", strategy.String())

	// Discover required images for this architecture
	images, err := m.discoverImages(ctx, source, arch, strategy, sshRunner, controllerNode)
	if err != nil {
		return fmt.Errorf("failed to discover required images: %w", err)
	}

	log.Infof("Discovered %d required images for %s on %s", len(images), source.Type, arch)

	// Ensure each image is cached and available
	for _, image := range images {
		if err := m.EnsureImage(ctx, image, sshRunner, nodeName, controllerNode); err != nil {
			log.Infof("Warning: Failed to ensure image %s: %v", image, err)
			// Continue with other images instead of failing completely
		}
	}

	return nil
}

// EnsureImage ensures a specific image is cached and available on the target node
func (m *ImageManager) EnsureImage(ctx context.Context, image string, sshRunner interfaces.SSHRunner, nodeName string, controllerNode string) error {
	// Initialize tool availability if not already done
	if m.availability == nil {
		if err := m.initializeToolAvailability(ctx, sshRunner, controllerNode); err != nil {
			return fmt.Errorf("failed to initialize tool availability: %w", err)
		}
	}

	// Get node architecture
	arch, err := m.getNodeArchitecture(ctx, sshRunner, nodeName)
	if err != nil {
		return fmt.Errorf("failed to determine node architecture: %w", err)
	}

	// Parse image reference
	ref := ParseImageReference(image, arch)
	log.Infof("Ensuring image %s is available for architecture %s", ref.String(), arch)

	// Check if image is already cached
	cacheKey := ref.CacheKey()
	if m.IsImageCached(cacheKey) {
		log.Infof("Image %s is already cached locally", ref.String())
	} else {
		// Determine optimal strategy for pulling
		strategy := m.toolDetector.DetermineOptimalStrategy(m.availability, m.config)

		// Pull and cache the image using the appropriate strategy
		if err := m.pullAndCacheImageWithStrategy(ctx, ref, strategy, sshRunner, controllerNode); err != nil {
			return fmt.Errorf("failed to pull and cache image: %w", err)
		}
	}

	// Upload image to target node
	if err := m.UploadImageToNode(ctx, ref, sshRunner, nodeName); err != nil {
		return fmt.Errorf("failed to upload image to node: %w", err)
	}

	return nil
}

// IsImageCached checks if an image is already cached locally
func (m *ImageManager) IsImageCached(cacheKey string) bool {
	imageInfo, exists := m.imageIndex.Images[cacheKey]
	if !exists {
		return false
	}

	// Check if the file actually exists
	if _, err := os.Stat(imageInfo.LocalPath); os.IsNotExist(err) {
		// Remove from index if file doesn't exist
		delete(m.imageIndex.Images, cacheKey)
		m.saveIndex()
		return false
	}

	// Update last accessed time
	imageInfo.LastAccessed = time.Now()
	m.imageIndex.Images[cacheKey] = imageInfo
	m.saveIndex()

	return true
}

// GetLocalImagePath returns the local path of a cached image
func (m *ImageManager) GetLocalImagePath(cacheKey string) (string, error) {
	imageInfo, exists := m.imageIndex.Images[cacheKey]
	if !exists {
		return "", fmt.Errorf("image %s not found in cache", cacheKey)
	}
	return imageInfo.LocalPath, nil
}

// getNodeArchitecture determines the architecture of a node
func (m *ImageManager) getNodeArchitecture(ctx context.Context, sshRunner interfaces.SSHRunner, nodeName string) (string, error) {
	// Execute command to get architecture
	cmd := "uname -m"
	output, err := sshRunner.RunCommand(nodeName, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to determine node architecture: %w", err)
	}

	arch := strings.TrimSpace(output)

	// Standardize architecture names
	switch arch {
	case "x86_64":
		return "amd64", nil
	case "aarch64":
		return "arm64", nil
	default:
		return arch, nil
	}
}

// pullAndCacheImageOnController pulls an image on controller node and downloads it to local cache
func (m *ImageManager) pullAndCacheImageOnController(ctx context.Context, ref ImageReference, sshRunner interfaces.SSHRunner, controllerNode string) error {
	log.Infof("Pulling and caching image: %s for %s on controller node", ref.String(), ref.Arch)

	// Normalized filename for cache storage
	normalizedName := ref.NormalizedName()
	localPath := filepath.Join(m.cacheDir, normalizedName+".tar")
	remotePath := fmt.Sprintf("/tmp/%s.tar", normalizedName)

	// Build script to pull and save image on controller node
	script := fmt.Sprintf(`#!/bin/bash
set -e

# Check if nerdctl is available, fallback to docker
if command -v nerdctl >/dev/null 2>&1; then
    CONTAINER_CMD="nerdctl"
elif command -v docker >/dev/null 2>&1; then
    CONTAINER_CMD="docker"
else
    echo "Error: Neither nerdctl nor docker found"
    exit 1
fi

# Pull image with platform specification
echo "Pulling image %s for platform linux/%s..."
$CONTAINER_CMD pull --platform linux/%s %s

# Save image to tar file (already compressed)
echo "Saving image to %s..."
$CONTAINER_CMD save -o %s %s

# Get file size for logging
ls -la %s
echo "Image saved successfully"
`, ref.String(), ref.Arch, ref.Arch, ref.String(), remotePath, remotePath, ref.String(), remotePath)

	// Execute script on controller node
	if _, err := sshRunner.RunCommand(controllerNode, script); err != nil {
		return fmt.Errorf("failed to pull and save image on controller node: %w", err)
	}

	// Download the image file from controller node to local cache
	if err := sshRunner.DownloadFile(controllerNode, remotePath, localPath); err != nil {
		return fmt.Errorf("failed to download image from controller node: %w", err)
	}

	// Clean up remote file
	cleanupCmd := fmt.Sprintf("rm -f %s", remotePath)
	if _, err := sshRunner.RunCommand(controllerNode, cleanupCmd); err != nil {
		log.Infof("Warning: Failed to cleanup remote image file: %v", err)
	}

	// Get file size
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to get image file size: %w", err)
	}
	fileSize := fileInfo.Size()

	// Update the index
	m.imageIndex.Images[ref.CacheKey()] = ImageInfo{
		Reference:     ref,
		LocalPath:     localPath,
		Size:          fileSize,
		LastAccessed:  time.Now(),
		LastUpdated:   time.Now(),
		OriginalSize:  fileSize, // Same as size since no additional compression
		Architectures: []string{ref.Arch},
	}

	// Save the updated index
	if err := m.saveIndex(); err != nil {
		return fmt.Errorf("failed to save image index: %w", err)
	}

	log.Infof("Successfully cached image %s for %s (%d bytes)", ref.String(), ref.Arch, fileSize)
	return nil
}

// UploadImageToNode uploads a cached image to a target node
func (m *ImageManager) UploadImageToNode(ctx context.Context, ref ImageReference, sshRunner interfaces.SSHRunner, nodeName string) error {
	log.Infof("Uploading image %s to node %s", ref.String(), nodeName)

	cacheKey := ref.CacheKey()
	imageInfo, exists := m.imageIndex.Images[cacheKey]
	if !exists {
		return fmt.Errorf("image %s not found in cache", ref.String())
	}

	// Check if local file exists
	if _, err := os.Stat(imageInfo.LocalPath); os.IsNotExist(err) {
		return fmt.Errorf("local image file not found: %s", imageInfo.LocalPath)
	}

	// Create remote directory
	remoteDir := "/usr/local/src/images"
	createDirCmd := fmt.Sprintf("sudo mkdir -p %s", remoteDir)
	if _, err := sshRunner.RunCommand(nodeName, createDirCmd); err != nil {
		return fmt.Errorf("failed to create remote directory: %w", err)
	}

	// Remote path for the image
	remotePath := filepath.Join(remoteDir, filepath.Base(imageInfo.LocalPath))

	// Check if image already exists on remote node and is loaded in containerd
	checkImageCmd := fmt.Sprintf("sudo ctr -n=k8s.io images ls name==%s", ref.String())
	output, err := sshRunner.RunCommand(nodeName, checkImageCmd)
	if err == nil && strings.Contains(output, ref.String()) {
		log.Infof("Image %s already loaded on node %s", ref.String(), nodeName)
		return nil
	}

	// Check if image file exists on remote node
	checkFileCmd := fmt.Sprintf("test -f %s && echo 'exists' || echo 'not_exists'", remotePath)
	output, err = sshRunner.RunCommand(nodeName, checkFileCmd)
	if err != nil {
		return fmt.Errorf("failed to check if image file exists on remote node: %w", err)
	}

	if strings.TrimSpace(output) != "exists" {
		// Upload the image file
		if err := sshRunner.UploadFile(nodeName, imageInfo.LocalPath, remotePath); err != nil {
			return fmt.Errorf("failed to upload image file: %w", err)
		}
	}

	// Load the image into containerd (no decompression needed since tar files are already compressed)
	loadCmd := fmt.Sprintf("sudo ctr -n=k8s.io images import %s", remotePath)
	if _, err := sshRunner.RunCommand(nodeName, loadCmd); err != nil {
		return fmt.Errorf("failed to load image on remote node: %w", err)
	}

	// Update last accessed time
	imageInfo.LastAccessed = time.Now()
	m.imageIndex.Images[cacheKey] = imageInfo
	if err := m.saveIndex(); err != nil {
		log.Infof("Warning: Failed to update image index: %v", err)
	}

	log.Infof("Successfully uploaded and loaded image %s on node %s", ref.String(), nodeName)
	return nil
}

// CleanupOldImages removes images older than the specified duration
func (m *ImageManager) CleanupOldImages(maxAge time.Duration) error {
	log.Infof("Cleaning up images older than %v", maxAge)

	cutoff := time.Now().Add(-maxAge)
	var toDelete []string

	for cacheKey, imageInfo := range m.imageIndex.Images {
		if imageInfo.LastAccessed.Before(cutoff) {
			toDelete = append(toDelete, cacheKey)
		}
	}

	for _, cacheKey := range toDelete {
		imageInfo := m.imageIndex.Images[cacheKey]
		log.Infof("Removing old image: %s", imageInfo.Reference.String())

		// Remove file
		if err := os.Remove(imageInfo.LocalPath); err != nil && !os.IsNotExist(err) {
			log.Infof("Warning: Failed to remove image file %s: %v", imageInfo.LocalPath, err)
		}

		// Remove from index
		delete(m.imageIndex.Images, cacheKey)
	}

	if len(toDelete) > 0 {
		log.Infof("Cleaned up %d old images", len(toDelete))
		return m.saveIndex()
	}

	return nil
}

// GetCacheStats returns statistics about the image cache
func (m *ImageManager) GetCacheStats() (int, int64) {
	var totalSize int64
	count := len(m.imageIndex.Images)

	for _, imageInfo := range m.imageIndex.Images {
		totalSize += imageInfo.Size
	}

	return count, totalSize
}

// initializeToolAvailability detects and caches tool availability
func (m *ImageManager) initializeToolAvailability(ctx context.Context, sshRunner interfaces.SSHRunner, controllerNode string) error {
	log.Infof("Detecting tool availability...")

	availability, err := m.toolDetector.DetectToolAvailability(ctx, sshRunner, controllerNode)
	if err != nil {
		return fmt.Errorf("failed to detect tool availability: %w", err)
	}

	m.availability = availability
	m.toolDetector.LogToolAvailability(availability)

	return nil
}

// discoverImages discovers required images using the appropriate strategy
func (m *ImageManager) discoverImages(ctx context.Context, source ImageSource, arch string, strategy ImageManagementStrategy, sshRunner interfaces.SSHRunner, controllerNode string) ([]string, error) {
	switch strategy {
	case ImageManagementLocal:
		// Use local tools for discovery
		return m.discoverImagesLocally(ctx, source, arch)
	case ImageManagementController, ImageManagementTarget:
		// Use controller node for discovery
		return m.imageDiscovery.GetRequiredImagesForArch(ctx, source, arch, sshRunner, controllerNode)
	default:
		return nil, fmt.Errorf("unsupported image management strategy: %s", strategy.String())
	}
}

// discoverImagesLocally discovers images using local tools
func (m *ImageManager) discoverImagesLocally(ctx context.Context, source ImageSource, arch string) ([]string, error) {
	// Create a local image discovery instance that uses local tools
	localDiscovery := NewLocalImageDiscovery()
	return localDiscovery.GetRequiredImagesForArch(ctx, source, arch)
}

// pullAndCacheImageWithStrategy pulls and caches an image using the specified strategy
func (m *ImageManager) pullAndCacheImageWithStrategy(ctx context.Context, ref ImageReference, strategy ImageManagementStrategy, sshRunner interfaces.SSHRunner, controllerNode string) error {
	log.Infof("Pulling and caching image %s using %s strategy", ref.String(), strategy.String())

	switch strategy {
	case ImageManagementLocal:
		return m.pullAndCacheImageLocally(ctx, ref)
	case ImageManagementController:
		return m.pullAndCacheImageOnController(ctx, ref, sshRunner, controllerNode)
	case ImageManagementTarget:
		// For target strategy, we would pull directly on target nodes
		// This is more complex and might be implemented later
		return fmt.Errorf("target strategy not yet implemented, falling back to controller strategy")
	default:
		return fmt.Errorf("unsupported image management strategy: %s", strategy.String())
	}
}

// pullAndCacheImageLocally pulls an image locally and caches it
func (m *ImageManager) pullAndCacheImageLocally(ctx context.Context, ref ImageReference) error {
	log.Infof("Pulling and caching image locally: %s for %s", ref.String(), ref.Arch)

	// Normalized filename for cache storage
	normalizedName := ref.NormalizedName()
	outputPath := filepath.Join(m.cacheDir, normalizedName+".tar")

	// Get preferred container tool
	containerTool := m.toolDetector.GetPreferredContainerTool(ImageManagementLocal, m.availability)

	// Build pull command with platform specification
	platform := fmt.Sprintf("linux/%s", ref.Arch)
	cmd := exec.CommandContext(ctx, containerTool, "pull", "--platform", platform, ref.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pull image locally: %w", err)
	}

	// Save image to tar file (images exported by nerdctl/docker are already compressed)
	cmd = exec.CommandContext(ctx, containerTool, "save", "-o", outputPath, ref.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to save image locally: %w", err)
	}

	// Get file size
	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("failed to get image file size: %w", err)
	}
	fileSize := fileInfo.Size()

	// Update the index
	m.imageIndex.Images[ref.CacheKey()] = ImageInfo{
		Reference:     ref,
		LocalPath:     outputPath,
		Size:          fileSize,
		LastAccessed:  time.Now(),
		LastUpdated:   time.Now(),
		OriginalSize:  fileSize, // Same as size since no additional compression
		Architectures: []string{ref.Arch},
	}

	// Save the updated index
	if err := m.saveIndex(); err != nil {
		return fmt.Errorf("failed to save image index: %w", err)
	}

	log.Infof("Successfully cached image locally %s for %s (%d bytes)", ref.String(), ref.Arch, fileSize)
	return nil
}
