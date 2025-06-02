package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/monshunter/ohmykube/pkg/envar"
	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/utils"
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
	imageRecorder  interfaces.ImageRecorder
	cachedRecorder *imageCachedMarker
	archRecords    map[string]string
	lock           sync.RWMutex
}

type imageCachedMarker struct {
	cached sync.Map
}

func (c *imageCachedMarker) isMarkedCached(ref, nodeName string) bool {
	_, ok := c.cached.Load(c.key(ref, nodeName))
	return ok
}

func (c *imageCachedMarker) markAsCached(ref, nodeName string) {
	c.cached.Store(c.key(ref, nodeName), true)
}

func (c *imageCachedMarker) key(ref, nodeName string) string {
	return fmt.Sprintf("%s:%s", nodeName, ref)
}

var globalImageRecorder interfaces.ImageRecorder

func SetGlobalImageRecorder(imageRecorder interfaces.ImageRecorder) {
	globalImageRecorder = imageRecorder
}

// Singleton pattern implementation
var (
	instance     *ImageManager
	instanceOnce sync.Once
	instanceErr  error
)

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
		imageRecorder:  globalImageRecorder,
		cachedRecorder: &imageCachedMarker{},
		archRecords:    make(map[string]string),
	}

	// Load existing index or create new one
	if err := manager.loadIndex(); err != nil {
		return nil, fmt.Errorf("failed to load image index: %w", err)
	}

	return manager, nil
}

// GetImageManager returns the singleton instance of ImageManager
// This is the recommended way to get an ImageManager instance to avoid creating multiple
// identical objects during the same execution.
func GetImageManager() (*ImageManager, error) {
	instanceOnce.Do(func() {
		instance, instanceErr = NewImageManagerWithConfig(DefaultImageManagementConfig())
	})
	return instance, instanceErr
}

// SetImageRecorder sets the image recorder for image tracking
func (m *ImageManager) SetImageRecorder(imageRecorder interfaces.ImageRecorder) {
	m.imageRecorder = imageRecorder
}

// loadIndex loads the image index from disk or creates a new one
func (m *ImageManager) loadIndex() error {
	if _, err := os.Stat(m.indexPath); os.IsNotExist(err) {
		// Create new index
		m.imageIndex = &ImageIndex{
			Version:   "1.0",
			Images:    make([]ImageInfo, 0),
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
		return fmt.Errorf("failed to parse image index: %w", err)
	}

	// Initialize images slice if nil
	if m.imageIndex.Images == nil {
		m.imageIndex.Images = make([]ImageInfo, 0)
	}

	return nil
}

// saveIndex saves the image index to disk
func (m *ImageManager) saveIndex() error {
	imageIndex := m.imageIndex.Copy()
	imageIndex.UpdatedAt = time.Now()

	data, err := yaml.Marshal(imageIndex)
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	if err := os.WriteFile(m.indexPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}

	return nil
}

// EnsureImages ensures all required images are cached and available on the target node
func (m *ImageManager) EnsureImages(ctx context.Context, source ImageSource, sshRunner interfaces.SSHRunner, nodeName string, controllerNode string) error {
	// Initialize tool availability if not already done
	if err := m.initializeToolAvailability(ctx, sshRunner, controllerNode); err != nil {
		return fmt.Errorf("failed to initialize tool availability: %w", err)
	}

	// Get node architecture first
	arch, err := m.getNodeArchitecture(ctx, sshRunner, nodeName)
	if err != nil {
		return fmt.Errorf("failed to determine node architecture: %w", err)
	}

	// Determine optimal strategy using consistent tool preference (controller → target)
	strategy := m.toolDetector.DetermineOptimalStrategy(m.availability, m.config)
	log.Debugf("Using %s strategy for image management", strategy.String())

	// Check if the selected strategy can handle this specific source, if not, find a fallback
	if !m.toolDetector.CanStrategyHandleSource(strategy, source, m.availability) {
		log.Debugf("Primary strategy %s cannot handle %s source, finding fallback...", strategy.String(), source.Type)
		strategy = m.toolDetector.DetermineOptimalStrategyForSource(source, m.availability, m.config)
		log.Debugf("Using fallback strategy %s for %s source", strategy.String(), source.Type)
	}

	// Discover required images for this architecture
	images, err := m.discoverImages(ctx, source, strategy, sshRunner, controllerNode)
	if err != nil {
		return fmt.Errorf("failed to discover required images: %w", err)
	}

	log.Debugf("Discovered %d required images for %s on %s", len(images), source.Type, arch)

	// Ensure each image is cached and available
	for _, image := range images {
		if err := m.EnsureImage(ctx, image, sshRunner, nodeName, controllerNode); err != nil {
			log.Warningf("Failed to ensure image %s: %v", image, err)
			// Continue with other images instead of failing completely
		}
	}

	return nil
}

// EnsureImage ensures a specific image is cached and available on the target node
func (m *ImageManager) EnsureImage(ctx context.Context, image string, sshRunner interfaces.SSHRunner, nodeName string, controllerNode string) error {
	// Initialize tool availability if not already done
	if err := m.initializeToolAvailability(ctx, sshRunner, controllerNode); err != nil {
		return fmt.Errorf("failed to initialize tool availability: %w", err)
	}

	// Get node architecture
	arch, err := m.getNodeArchitecture(ctx, sshRunner, nodeName)
	if err != nil {
		return fmt.Errorf("failed to determine node architecture: %w", err)
	}

	// Parse image reference
	ref := ParseImageReference(image, arch)
	log.Debugf("Ensuring image %s is available for architecture %s on node %s", ref.String(), arch, nodeName)

	// Check if image is already cached for this specific architecture
	cacheKey := ref.CacheKey()
	if m.IsImageCached(cacheKey) {
		// Verify the cached image is for the correct architecture
		imageInfo, _ := m.imageIndex.findImageByName(cacheKey)
		if imageInfo != nil && imageInfo.Reference.Arch == arch {
			log.Debugf("Image %s is already cached locally for architecture %s", ref.String(), arch)
		} else {
			log.Debugf("Image %s cached for different architecture (%s), need to cache for %s", ref.String(), imageInfo.Reference.Arch, arch)
			// Remove the incorrectly cached image and re-cache for correct architecture
			m.imageIndex.removeImageByName(cacheKey)
			m.saveIndex()
		}
	}

	// Check again after potential cleanup
	if !m.IsImageCached(cacheKey) {
		// For individual images, we need a container runtime strategy
		// Create a generic source for container image pulling
		source := ImageSource{
			Type: "container", // Generic container image
		}

		// Determine optimal strategy for pulling based on container requirements
		strategy := m.toolDetector.DetermineOptimalStrategyForSource(source, m.availability, m.config)

		// Pull and cache the image using the appropriate strategy
		if err := m.pullAndCacheImageWithStrategy(ctx, ref, strategy, sshRunner, controllerNode); err != nil {
			return fmt.Errorf("failed to pull and cache image: %w", err)
		}
	}
	if m.imageRecorder != nil {
		m.imageRecorder.RecordImage(ref.CacheKey())
	}
	// Upload image to target node
	if err := m.UploadImageToNode(ctx, ref, sshRunner, nodeName); err != nil {
		return fmt.Errorf("failed to upload image to node: %w", err)
	}

	return nil
}

// IsImageCached checks if an image is already cached locally
func (m *ImageManager) IsImageCached(cacheKey string) bool {
	imageInfo, _ := m.imageIndex.findImageByName(cacheKey)
	if imageInfo == nil {
		return false
	}

	// Check if the file actually exists
	if _, err := os.Stat(imageInfo.LocalPath); os.IsNotExist(err) {
		// Remove from index if file doesn't exist
		m.imageIndex.removeImageByName(cacheKey)
		m.saveIndex()
		return false
	}

	// Update last accessed time
	imageInfo.LastAccessed = time.Now()
	m.imageIndex.updateOrAddImage(cacheKey, *imageInfo)
	m.saveIndex()

	return true
}

// getNodeArchitecture determines the architecture of a node
func (m *ImageManager) getNodeArchitecture(ctx context.Context, sshRunner interfaces.SSHRunner, nodeName string) (string, error) {
	m.lock.RLock()
	if arch, ok := m.archRecords[nodeName]; ok {
		m.lock.RUnlock()
		return arch, nil
	}
	m.lock.RUnlock()

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
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	}
	m.lock.Lock()
	m.archRecords[nodeName] = arch
	m.lock.Unlock()
	return arch, nil
}

// pullAndCacheImageOnController pulls an image on controller node and downloads it to local cache
func (m *ImageManager) pullAndCacheImageOnController(ctx context.Context, ref ImageReference, sshRunner interfaces.SSHRunner, controllerNode string) error {
	log.Debugf("Pulling and caching image: %s for %s on controller node", ref.String(), ref.Arch)

	// Normalized filename for cache storage
	normalizedName := ref.NormalizedName()
	localPath := filepath.Join(m.cacheDir, normalizedName+".tar")
	remotePath := fmt.Sprintf("/tmp/%s.tar", normalizedName)

	// Build script to pull and save image on controller node
	script := fmt.Sprintf(`#!/bin/bash
set -e

# Set variables to avoid repetition
IMAGE_REF="%s"
ARCH="%s"
PLATFORM="linux/${ARCH}"
OUTPUT_PATH="%s"

# Function to log with timestamp
log_with_time() {
    echo "[$(date '+%%Y-%%m-%%d %%H:%%M:%%S')] $1"
}

# Use nerdctl for reliable multi-architecture image handling
if command -v nerdctl >/dev/null 2>&1; then
    log_with_time "Using nerdctl for container operations"

    # Pull image with platform specification
    log_with_time "Pulling image ${IMAGE_REF} for platform ${PLATFORM}..."
    timeout 300 nerdctl --namespace=k8s.io pull --platform "${PLATFORM}" "${IMAGE_REF}" || {
        log_with_time "ERROR: Pull operation timed out or failed"
        exit 1
    }
    log_with_time "Pull completed successfully"

    # Save image with platform specification
    log_with_time "Saving image to ${OUTPUT_PATH} using nerdctl for platform ${PLATFORM}..."
    timeout 600 nerdctl --namespace=k8s.io save --platform "${PLATFORM}" -o "${OUTPUT_PATH}" "${IMAGE_REF}" || {
        log_with_time "ERROR: Save operation timed out or failed"
        exit 1
    }
    log_with_time "Save completed successfully"

else
    log_with_time "ERROR: nerdctl not found. nerdctl is required for reliable multi-architecture image handling."
    log_with_time "Please install nerdctl on the controller node."
    exit 1
fi

# Get file size for logging
log_with_time "Checking saved file..."
ls -la "${OUTPUT_PATH}"
FILE_SIZE=$(stat -c%%s "${OUTPUT_PATH}" 2>/dev/null || echo "0")
log_with_time "Image saved successfully (${FILE_SIZE} bytes)"

# Verify file is not suspiciously small
if [ "${FILE_SIZE}" -lt 102400 ]; then
    log_with_time "WARNING: Saved file is very small (${FILE_SIZE} bytes), this might indicate an issue"
fi
`, ref.String(), ref.Arch, remotePath)

	// Execute script on controller node
	log.Debugf("Executing image pull and save script on controller node...")
	if _, err := sshRunner.RunCommand(controllerNode, script); err != nil {
		return fmt.Errorf("failed to pull and save image on controller node: %w", err)
	}
	log.Debugf("Image pull and save completed on controller node")

	// Verify the remote file exists and get its size before downloading
	log.Debugf("Verifying remote file exists: %s", remotePath)
	checkCmd := fmt.Sprintf("ls -la %s && stat -c%%s %s", remotePath, remotePath)
	output, err := sshRunner.RunCommand(controllerNode, checkCmd)
	if err != nil {
		return fmt.Errorf("failed to verify remote file exists: %w", err)
	}
	log.Debugf("Remote file verification: %s", strings.TrimSpace(output))

	// Download the image file from controller node to local cache
	log.Debugf("Downloading image file from controller node: %s -> %s", remotePath, localPath)
	if err := sshRunner.DownloadFile(controllerNode, remotePath, localPath); err != nil {
		return fmt.Errorf("failed to download image from controller node: %w", err)
	}
	log.Debugf("Image file download completed")

	// Clean up remote file
	cleanupCmd := fmt.Sprintf("rm -f %s", remotePath)
	if _, err := sshRunner.RunCommand(controllerNode, cleanupCmd); err != nil {
		log.Warningf("Failed to cleanup remote image file: %v", err)
	}

	// Get file size and verify download
	log.Debugf("Verifying downloaded file: %s", localPath)
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to get image file size: %w", err)
	}
	fileSize := fileInfo.Size()
	log.Debugf("Downloaded file: name=%s, size=%s", fileInfo.Name(), utils.FormatSize(fileSize))

	// Warn if file size seems too small
	if fileSize < 100*1024 {
		log.Warningf("Downloaded image file is very small (%s), this might indicate an issue", utils.FormatSize(fileSize))
	}

	// Update the index
	log.Debugf("Updating image index for %s", ref.String())
	cacheKey := ref.CacheKey()
	m.imageIndex.updateOrAddImage(cacheKey, ImageInfo{
		Reference:     ref,
		LocalPath:     localPath,
		Size:          fileSize,
		LastAccessed:  time.Now(),
		LastUpdated:   time.Now(),
		OriginalSize:  fileSize, // Same as size since no additional compression
		Architectures: []string{ref.Arch},
	})

	// Save the updated index
	log.Debugf("Saving updated image index...")
	if err := m.saveIndex(); err != nil {
		return fmt.Errorf("failed to save image index: %w", err)
	}
	log.Debugf("Image index saved successfully")
	log.Debugf("Successfully cached image %s for %s (%s)", ref.String(), ref.Arch, utils.FormatSize(fileSize))
	return nil
}

// UploadImageToNode uploads a cached image to a target node
func (m *ImageManager) UploadImageToNode(ctx context.Context, ref ImageReference, sshRunner interfaces.SSHRunner, nodeName string) error {
	if m.cachedRecorder != nil {
		if m.cachedRecorder.isMarkedCached(ref.CacheKey(), nodeName) {
			log.Debugf("Image %s already cached on node %s, skipping upload", ref.String(), nodeName)
			return nil
		}
	}

	log.Debugf("Uploading image %s (linux/%s) to node %s", ref.String(), ref.Arch, nodeName)

	cacheKey := ref.CacheKey()
	imageInfo, _ := m.imageIndex.findImageByName(cacheKey)
	if imageInfo == nil {
		return fmt.Errorf("image %s not found in cache", ref.String())
	}

	// Verify the cached image is for the correct architecture
	if imageInfo.Reference.Arch != ref.Arch {
		return fmt.Errorf("cached image %s is for architecture %s, but need %s", ref.String(), imageInfo.Reference.Arch, ref.Arch)
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
	// Include platform specification in the check
	checkImageCmd := fmt.Sprintf("sudo ctr -n=k8s.io images ls name==%s | grep 'linux/%s' || true", ref.String(), ref.Arch)
	output, err := sshRunner.RunCommand(nodeName, checkImageCmd)
	if err == nil && strings.Contains(output, ref.String()) && strings.Contains(output, fmt.Sprintf("linux/%s", ref.Arch)) {
		log.Debugf("Image %s (linux/%s) already loaded on node %s", ref.String(), ref.Arch, nodeName)
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

	// Load the image into containerd using appropriate tool-specific commands
	loadScript := fmt.Sprintf(`#!/bin/bash
set -e

# Set variables to avoid repetition
IMAGE_REF="%s"
ARCH="%s"
PLATFORM="linux/${ARCH}"
IMAGE_FILE="%s"

# Function to log with timestamp
log_with_time() {
    echo "[$(date '+%%Y-%%m-%%d %%H:%%M:%%S')] $1"
}

# Use nerdctl or ctr for reliable container operations
if command -v nerdctl >/dev/null 2>&1; then
    log_with_time "Loading image using nerdctl for platform ${PLATFORM}..."
    # nerdctl load command with namespace and platform
    timeout 300 sudo nerdctl --namespace=k8s.io load -i "${IMAGE_FILE}" --platform "${PLATFORM}" || {
        log_with_time "ERROR: nerdctl load operation timed out or failed"
        exit 1
    }
    log_with_time "nerdctl load completed successfully"

elif command -v ctr >/dev/null 2>&1; then
    log_with_time "Loading image using ctr for platform ${PLATFORM}..."
    # ctr import command with namespace (platform is auto-detected from tar)
    timeout 300 sudo ctr --namespace=k8s.io images import "${IMAGE_FILE}" || {
        log_with_time "ERROR: ctr import operation timed out or failed"
        exit 1
    }
    log_with_time "ctr import completed successfully"

else
    log_with_time "ERROR: Neither nerdctl nor ctr found. One of these tools is required for reliable container operations."
    log_with_time "Please install nerdctl or ensure containerd is properly configured."
    exit 1
fi

# Verify the image was loaded using ctr
log_with_time "Verifying image was loaded..."
if sudo ctr -n=k8s.io images ls name=="${IMAGE_REF}" | grep "${PLATFORM}"; then
    log_with_time "Image verification successful"
else
    log_with_time "WARNING: Image verification failed or platform mismatch"
fi
`, ref.String(), ref.Arch, remotePath)

	if _, err := sshRunner.RunCommand(nodeName, loadScript); err != nil {
		return fmt.Errorf("failed to load image on remote node: %w", err)
	}

	// Update last accessed time
	imageInfo.LastAccessed = time.Now()
	m.imageIndex.updateOrAddImage(cacheKey, *imageInfo)
	if m.cachedRecorder != nil {
		m.cachedRecorder.markAsCached(ref.CacheKey(), nodeName)
	}
	if err := m.saveIndex(); err != nil {
		log.Warningf("Failed to update image index: %v", err)
	}

	log.Debugf("Successfully uploaded and loaded image %s on node %s", ref.String(), nodeName)
	return nil
}

// initializeToolAvailability detects and caches tool availability
func (m *ImageManager) initializeToolAvailability(ctx context.Context, sshRunner interfaces.SSHRunner, controllerNode string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Check if already initialized
	if m.availability != nil {
		return nil
	}

	log.Debugf("Detecting tool availability...")
	availability, err := m.toolDetector.DetectToolAvailability(ctx, sshRunner, controllerNode)
	if err != nil {
		return fmt.Errorf("failed to detect tool availability: %w", err)
	}

	m.availability = availability
	m.toolDetector.LogToolAvailability(availability)

	return nil
}

// discoverImages discovers required images using the appropriate strategy
func (m *ImageManager) discoverImages(ctx context.Context, source ImageSource, strategy ImageManagementStrategy, sshRunner interfaces.SSHRunner, controllerNode string) ([]string, error) {
	switch strategy {
	case ImageManagementController, ImageManagementTarget:
		// Use controller node for discovery
		return m.imageDiscovery.GetRequiredImages(ctx, source, sshRunner, controllerNode)
	default:
		return nil, fmt.Errorf("unsupported image management strategy: %s", strategy.String())
	}
}

// pullAndCacheImageWithStrategy pulls and caches an image using the specified strategy
func (m *ImageManager) pullAndCacheImageWithStrategy(ctx context.Context, ref ImageReference, strategy ImageManagementStrategy, sshRunner interfaces.SSHRunner, controllerNode string) error {
	log.Debugf("Pulling and caching image %s using %s strategy", ref.String(), strategy.String())

	switch strategy {
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

// ReCacheClusterImages caches all cluster images to the target node for preheating
func (m *ImageManager) ReCacheClusterImages(sshRunner SSHRunner) error {
	controllerNode := m.imageRecorder.GetMasterName()
	workerNodes := m.imageRecorder.GetWorkerNames()
	return m.CacheClusterImagesForNodes(append([]string{controllerNode}, workerNodes...), sshRunner)
}

// CacheClusterImagesForNodes caches all cluster images to the target node for preheating
func (m *ImageManager) CacheClusterImagesForNodes(nodeNames []string, sshRunner SSHRunner) error {
	controllerNode := m.imageRecorder.GetMasterName()
	var wg sync.WaitGroup
	wg.Add(len(nodeNames))
	for _, nodeName := range nodeNames {
		go func(nodeName string) {
			defer wg.Done()
			if err := m.cacheClusterImages(controllerNode, nodeName, sshRunner); err != nil {
				log.Warningf("Failed to preheat cluster images for node %s: %v", nodeName, err)
				// Continue with other nodes even if one fails
			}
		}(nodeName)
	}
	wg.Wait()
	return nil
}

// CacheClusterImages caches all cluster images to the target node for preheating
func (m *ImageManager) cacheClusterImages(controllerNode string, nodeName string, sshRunner SSHRunner) error {
	// Check if image manager is set
	if m.imageRecorder == nil {
		log.Warning("No image manager set, skipping cluster image preheating")
		return nil
	}

	imageNames := m.imageRecorder.GetImageNames()
	if len(imageNames) == 0 {
		log.Debug("No cluster images to preheat")
		return nil
	}

	log.Debugf("Preheating %d cluster images to node %s", len(imageNames), nodeName)

	// Create image manager for caching
	ctx := context.Background()

	// Cache each cluster image with concurrent processing
	// Use a semaphore to limit concurrent uploads per node to avoid overwhelming the network
	maxConcurrentUploads := 3
	semaphore := make(chan struct{}, maxConcurrentUploads)
	var wg sync.WaitGroup

	for _, imageName := range imageNames {
		if len(imageName) == 0 {
			continue
		}

		image, _ := m.imageIndex.findImageByName(imageName)
		if image == nil {
			log.Warningf("Image %s not found in cache, skipping preheating", imageName)
			continue
		}

		wg.Add(1)
		go func(imgName string) {
			defer func() {
				wg.Done()
				<-semaphore
			}()
			// Acquire semaphore
			semaphore <- struct{}{}

			originalName := image.Reference.Original
			log.Debugf("Preheating image: %s", originalName)
			if err := m.EnsureImage(ctx, originalName, sshRunner, nodeName, controllerNode); err != nil {
				log.Warningf("Failed to preheat image %s: %v", originalName, err)
				// Continue with other images even if one fails
			}
		}(imageName)
	}

	wg.Wait()

	log.Debugf("Completed cluster image preheating for node %s", nodeName)
	return nil
}
