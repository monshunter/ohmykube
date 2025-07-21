package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/monshunter/ohmykube/pkg/envar"
	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
	"gopkg.in/yaml.v3"
)

// PackageManager implements the PackageCachePackageManager interface
type PackageManager struct {
	cacheDir    string
	indexPath   string
	downloader  *Downloader
	compressor  *Compressor
	index       *PackageIndex
	definitions map[string]PackageDefinition
}

// NewPackageManager creates a new cache manager instance
func NewPackageManager() (*PackageManager, error) {
	// Get cache directory from environment
	cacheDir := filepath.Join(envar.OhMyKubeCacheDir(), "packages")
	indexPath := filepath.Join(cacheDir, "index.yaml")

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Create downloader
	downloader := NewDownloader()

	// Create compressor
	compressor, err := NewCompressor()
	if err != nil {
		return nil, fmt.Errorf("failed to create compressor: %w", err)
	}

	manager := &PackageManager{
		cacheDir:    cacheDir,
		indexPath:   indexPath,
		downloader:  downloader,
		compressor:  compressor,
		definitions: GetPackageDefinitions(),
	}

	// Load existing index or create new one
	if err := manager.loadIndex(); err != nil {
		return nil, fmt.Errorf("failed to load package index: %w", err)
	}

	return manager, nil
}

// Close closes the manager and releases resources
func (m *PackageManager) Close() {
	if m.compressor != nil {
		m.compressor.Close()
	}
}

// loadIndex loads the package index from disk or creates a new one
func (m *PackageManager) loadIndex() error {
	if _, err := os.Stat(m.indexPath); os.IsNotExist(err) {
		// Create new index
		m.index = &PackageIndex{
			Version:   "1.0",
			Packages:  make([]PackageInfo, 0),
			UpdatedAt: time.Now(),
		}
		return m.saveIndex()
	}

	// Load existing index
	data, err := os.ReadFile(m.indexPath)
	if err != nil {
		return fmt.Errorf("failed to read index file: %w", err)
	}

	// Try to parse as new array format first
	m.index = &PackageIndex{}
	if err := yaml.Unmarshal(data, m.index); err != nil {
		return fmt.Errorf("failed to parse package index: %w", err)
	}

	// Initialize packages slice if nil
	if m.index.Packages == nil {
		m.index.Packages = make([]PackageInfo, 0)
	}

	return nil
}

// saveIndex saves the package index to disk
func (m *PackageManager) saveIndex() error {
	m.index.UpdatedAt = time.Now()

	data, err := yaml.Marshal(m.index)
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	if err := os.WriteFile(m.indexPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}

	return nil
}

// OldPackageIndex represents the old map-based index format for migration
type OldPackageIndex struct {
	Version   string                 `yaml:"version"`
	Packages  map[string]PackageInfo `yaml:"packages"`
	UpdatedAt time.Time              `yaml:"updated_at"`
}

// findPackage finds a package in the index by name, version, and architecture
func (m *PackageManager) findPackage(name, version, arch string) (*PackageInfo, int) {
	for i, pkg := range m.index.Packages {
		if pkg.Name == name && pkg.Version == version && pkg.Architecture == arch {
			return &pkg, i
		}
	}
	return nil, -1
}

// addPackage adds a new package to the index
func (m *PackageManager) addPackage(packageInfo PackageInfo) {
	m.index.Packages = append(m.index.Packages, packageInfo)
}

// updatePackage updates an existing package in the index
func (m *PackageManager) updatePackage(index int, packageInfo PackageInfo) {
	if index >= 0 && index < len(m.index.Packages) {
		m.index.Packages[index] = packageInfo
	}
}

// removePackage removes a package from the index by index
func (m *PackageManager) removePackage(index int) {
	if index >= 0 && index < len(m.index.Packages) {
		m.index.Packages = append(m.index.Packages[:index], m.index.Packages[index+1:]...)
	}
}

// EnsurePackage ensures the specified package is available both locally and on target nodes
func (m *PackageManager) EnsurePackage(ctx context.Context, packageName, version, arch string, sshRunner interfaces.SSHRunner, nodeName string) error {
	log.Debugf("Ensuring package %s-%s-%s is available", packageName, version, arch)

	// Check if package is already cached locally
	if m.IsPackageCached(packageName, version, arch) {
		log.Debugf("Package %s-%s-%s is already cached locally", packageName, version, arch)
	} else {
		// Download and cache the package
		if err := m.downloadAndCachePackage(ctx, packageName, version, arch); err != nil {
			return fmt.Errorf("failed to download and cache package: %w", err)
		}
	}

	if err := m.uploadPackage(ctx, packageName, version, arch, sshRunner, nodeName); err != nil {
		return fmt.Errorf("failed to upload package to node via SCP: %w", err)
	}

	return nil
}

// UploadPackage uploads a cached package to a target node
func (m *PackageManager) uploadPackage(ctx context.Context, packageName, version, arch string, sshRunner interfaces.SSHRunner, nodeName string) error {
	log.Debugf("Uploading package %s-%s-%s to node %s ", packageName, version, arch, nodeName)

	// Get package info
	packageInfo, _ := m.findPackage(packageName, version, arch)
	if packageInfo == nil {
		key := PackageKey(packageName, version, arch)
		return fmt.Errorf("package %s not found in cache", key)
	}

	// Check if local file exists
	if _, err := os.Stat(packageInfo.LocalPath); os.IsNotExist(err) {
		return fmt.Errorf("local package file not found: %s", packageInfo.LocalPath)
	}

	// Check if package already exists on remote node and verify its integrity
	if m.isPackageValidOnRemote(sshRunner, nodeName, packageInfo) {
		log.Debugf("Package %s-%s-%s already exists and is valid on node %s", packageName, version, arch, nodeName)
		return nil
	}

	// Upload using proper SCP protocol
	if err := sshRunner.UploadFile(nodeName, packageInfo.LocalPath, packageInfo.GetRemotePath()); err != nil {
		return fmt.Errorf("failed to upload package file via SCP: %w", err)
	}

	log.Debugf("Successfully uploaded package %s-%s-%s to node %s using SCP", packageName, version, arch, nodeName)
	return nil
}

// isPackageValidOnRemote checks if a package exists on remote node and verifies its integrity
func (m *PackageManager) isPackageValidOnRemote(sshRunner interfaces.SSHRunner, nodeName string, packageInfo *PackageInfo) bool {
	remotePath := packageInfo.GetRemotePath()

	// First check if file exists
	checkCmd := fmt.Sprintf("test -f %s && echo 'exists' || echo 'not_exists'", remotePath)
	output, err := sshRunner.RunCommand(nodeName, checkCmd)
	if err != nil {
		log.Debugf("Failed to check if package exists on remote node: %v", err)
		return false
	}

	if strings.TrimSpace(output) != "exists" {
		log.Debugf("Package file does not exist on remote node: %s", remotePath)
		return false
	}

	// Check file size to verify integrity
	sizeCmd := fmt.Sprintf("stat -c%%s %s 2>/dev/null || echo '0'", remotePath)
	output, err = sshRunner.RunCommand(nodeName, sizeCmd)
	if err != nil {
		log.Debugf("Failed to get remote file size: %v", err)
		return false
	}

	remoteSize, err := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	if err != nil {
		log.Debugf("Failed to parse remote file size: %v", err)
		return false
	}

	// Get local file size for comparison
	localInfo, err := os.Stat(packageInfo.LocalPath)
	if err != nil {
		log.Debugf("Failed to get local file info: %v", err)
		return false
	}

	// Compare file sizes
	if remoteSize != localInfo.Size() {
		log.Debugf("File size mismatch: remote=%d, local=%d, removing incomplete file", remoteSize, localInfo.Size())
		// Remove the incomplete file
		removeCmd := fmt.Sprintf("rm -f %s", remotePath)
		if _, err := sshRunner.RunCommand(nodeName, removeCmd); err != nil {
			log.Warningf("Failed to remove incomplete file: %v", err)
		}
		return false
	}

	// Optionally verify checksum for extra integrity (can be expensive for large files)
	// For now, size check should be sufficient to detect incomplete transfers

	log.Debugf("Package file is valid on remote node: %s (size: %d bytes)", remotePath, remoteSize)
	return true
}

// IsPackageCached checks if a package is already cached locally
func (m *PackageManager) IsPackageCached(packageName, version, arch string) bool {
	packageInfo, index := m.findPackage(packageName, version, arch)
	if packageInfo == nil {
		return false
	}

	// Check if the file actually exists
	if _, err := os.Stat(packageInfo.LocalPath); os.IsNotExist(err) {
		// Remove from index if file doesn't exist
		m.removePackage(index)
		m.saveIndex()
		return false
	}

	// Update last accessed time
	packageInfo.LastAccessedAt = time.Now()
	m.updatePackage(index, *packageInfo)
	m.saveIndex()

	return true
}

// downloadAndCachePackage downloads a package and caches it locally
func (m *PackageManager) downloadAndCachePackage(ctx context.Context, packageName, version, arch string) error {
	log.Debugf("Downloading and caching package %s-%s-%s", packageName, version, arch)

	// Get package definition
	definition, exists := m.definitions[packageName]
	if !exists {
		return fmt.Errorf("unknown package: %s", packageName)
	}

	// Get download URL and filename
	downloadURL := definition.GetURL(version, arch)
	originalFilename := definition.GetFilename(version, arch)

	// Create temporary download path
	tempDir := filepath.Join(m.cacheDir, "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	tempPath := filepath.Join(tempDir, originalFilename)

	// Download the file
	if err := m.downloader.DownloadWithRetry(ctx, downloadURL, tempPath, 3); err != nil {
		return fmt.Errorf("failed to download package: %w", err)
	}

	// Calculate checksum
	checksum, err := m.downloader.CalculateChecksum(tempPath)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	// Get original file size
	originalSize, err := m.downloader.GetFileSize(tempPath)
	if err != nil {
		return fmt.Errorf("failed to get file size: %w", err)
	}

	// Create compressed package path
	compressedFilename := packageName + "-" + version + "-" + arch + ".tar.zst"
	compressedPath := filepath.Join(m.cacheDir, compressedFilename)

	// Normalize the package to tar.zst format
	if err := m.compressor.NormalizeToTarZst(tempPath, compressedPath, definition.Format); err != nil {
		return fmt.Errorf("failed to normalize package to tar.zst: %w", err)
	}

	// Get compressed file size
	compressedSize, err := m.downloader.GetFileSize(compressedPath)
	if err != nil {
		return fmt.Errorf("failed to get compressed file size: %w", err)
	}

	// Clean up temp file
	os.Remove(tempPath)

	// Create package info
	packageInfo := PackageInfo{
		Name:           packageName,
		Version:        version,
		Architecture:   arch,
		Type:           string(definition.Type),
		DownloadURL:    downloadURL,
		LocalPath:      compressedPath,
		CompressedSize: compressedSize,
		OriginalSize:   originalSize,
		Checksum:       checksum,
		CachedAt:       time.Now(),
		LastAccessedAt: time.Now(),
	}
	packageInfo.RemotePath = packageInfo.GetRemotePath()

	// Add to index
	m.addPackage(packageInfo)

	// Save index
	if err := m.saveIndex(); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}

	log.Debugf("Successfully cached package %s-%s-%s (compressed: %d bytes, original: %d bytes)",
		packageName, version, arch, compressedSize, originalSize)

	return nil
}

// CleanupOldPackages removes old cached packages to free up space
func (m *PackageManager) CleanupOldPackages(maxAge time.Duration) error {
	log.Debugf("Cleaning up packages older than %v", maxAge)

	cutoff := time.Now().Add(-maxAge)
	var toDelete []int

	for i, packageInfo := range m.index.Packages {
		if packageInfo.LastAccessedAt.Before(cutoff) {
			// Remove the file
			if err := os.Remove(packageInfo.LocalPath); err != nil && !os.IsNotExist(err) {
				log.Errorf("Failed to remove package file %s: %v", packageInfo.LocalPath, err)
			} else {
				key := PackageKey(packageInfo.Name, packageInfo.Version, packageInfo.Architecture)
				log.Debugf("Removed old package: %s", key)
				toDelete = append(toDelete, i)
			}
		}
	}

	// Remove from index (in reverse order to maintain indices)
	for i := len(toDelete) - 1; i >= 0; i-- {
		m.removePackage(toDelete[i])
	}

	if len(toDelete) > 0 {
		if err := m.saveIndex(); err != nil {
			return fmt.Errorf("failed to save index after cleanup: %w", err)
		}
	}

	log.Debugf("Cleanup completed, removed %d packages", len(toDelete))
	return nil
}

// ListCachedPackages returns a list of all cached packages
func (m *PackageManager) ListCachedPackages() []PackageInfo {
	// Return a copy of the packages slice
	packages := make([]PackageInfo, len(m.index.Packages))
	copy(packages, m.index.Packages)
	return packages
}

// GetCacheStats returns statistics about the cache
func (m *PackageManager) GetCacheStats() (int, int64, int64) {
	var totalPackages int
	var totalCompressedSize int64
	var totalOriginalSize int64

	for _, packageInfo := range m.index.Packages {
		totalPackages++
		totalCompressedSize += packageInfo.CompressedSize
		totalOriginalSize += packageInfo.OriginalSize
	}

	return totalPackages, totalCompressedSize, totalOriginalSize
}
