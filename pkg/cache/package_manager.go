package cache

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
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
		// If that fails, try to parse as old map format and migrate
		log.Infof("Migrating old index format to new array format")
		if err := m.migrateOldIndex(data); err != nil {
			return fmt.Errorf("failed to migrate old index format: %w", err)
		}
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

// migrateOldIndex migrates from the old map-based format to the new array-based format
func (m *PackageManager) migrateOldIndex(data []byte) error {
	oldIndex := &OldPackageIndex{}
	if err := yaml.Unmarshal(data, oldIndex); err != nil {
		return fmt.Errorf("failed to parse old index format: %w", err)
	}

	// Convert map to array
	m.index = &PackageIndex{
		Version:   oldIndex.Version,
		Packages:  make([]PackageInfo, 0, len(oldIndex.Packages)),
		UpdatedAt: oldIndex.UpdatedAt,
	}

	for _, pkg := range oldIndex.Packages {
		m.index.Packages = append(m.index.Packages, pkg)
	}

	// Save the migrated index
	log.Infof("Migrated %d packages from old format", len(m.index.Packages))
	return m.saveIndex()
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
func (m *PackageManager) EnsurePackage(ctx context.Context, packageName, version, arch string, sshRunner interfaces.SSHCommandRunner, nodeName string) error {
	log.Infof("Ensuring package %s-%s-%s is available", packageName, version, arch)

	// Check if package is already cached locally
	if m.IsPackageCached(packageName, version, arch) {
		log.Infof("Package %s-%s-%s is already cached locally", packageName, version, arch)
	} else {
		// Download and cache the package
		if err := m.downloadAndCachePackage(ctx, packageName, version, arch); err != nil {
			return fmt.Errorf("failed to download and cache package: %w", err)
		}
	}

	// Try to use the new SSH interface if available, otherwise fall back to old method
	if newSSHRunner, ok := sshRunner.(interfaces.SSHRunner); ok {
		// Use the new optimized upload method with SCP
		if err := m.UploadPackageWithSCP(ctx, packageName, version, arch, newSSHRunner, nodeName); err != nil {
			return fmt.Errorf("failed to upload package to node via SCP: %w", err)
		}
	} else {
		// Fall back to the old method
		if err := m.UploadPackageToNode(ctx, packageName, version, arch, sshRunner, nodeName); err != nil {
			return fmt.Errorf("failed to upload package to node: %w", err)
		}
	}

	return nil
}

// UploadPackageWithSCP uploads a cached package to a target node using SCP protocol
func (m *PackageManager) UploadPackageWithSCP(ctx context.Context, packageName, version, arch string, sshRunner interfaces.SSHRunner, nodeName string) error {
	log.Infof("Uploading package %s-%s-%s to node %s using SCP protocol", packageName, version, arch, nodeName)

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

	// Check if package already exists on remote node
	checkCmd := fmt.Sprintf("test -f %s && echo 'exists' || echo 'not_exists'", packageInfo.GetRemotePath())
	output, err := sshRunner.RunCommand(nodeName, checkCmd)
	if err != nil {
		return fmt.Errorf("failed to check if package exists on remote node: %w", err)
	}

	if strings.TrimSpace(output) == "exists" {
		log.Infof("Package %s-%s-%s already exists on node %s", packageName, version, arch, nodeName)
		return nil
	}

	// Upload using proper SCP protocol
	if err := sshRunner.UploadFile(nodeName, packageInfo.LocalPath, packageInfo.GetRemotePath()); err != nil {
		return fmt.Errorf("failed to upload package file via SCP: %w", err)
	}

	log.Infof("Successfully uploaded package %s-%s-%s to node %s using SCP", packageName, version, arch, nodeName)
	return nil
}

// EnsurePackageWithSCP ensures the specified package is available both locally and on target nodes using SCP
func (m *PackageManager) EnsurePackageWithSCP(ctx context.Context, packageName, version, arch string, sshRunner interfaces.SSHRunner, nodeName string) error {
	log.Infof("Ensuring package %s-%s-%s is available using SCP", packageName, version, arch)

	// Check if package is already cached locally
	if m.IsPackageCached(packageName, version, arch) {
		log.Infof("Package %s-%s-%s is already cached locally", packageName, version, arch)
	} else {
		// Download and cache the package
		if err := m.downloadAndCachePackage(ctx, packageName, version, arch); err != nil {
			return fmt.Errorf("failed to download and cache package: %w", err)
		}
	}

	// Upload package to target node using SCP
	if err := m.UploadPackageWithSCP(ctx, packageName, version, arch, sshRunner, nodeName); err != nil {
		return fmt.Errorf("failed to upload package to node via SCP: %w", err)
	}

	return nil
}

// GetLocalPackagePath returns the local path of a cached package
func (m *PackageManager) GetLocalPackagePath(packageName, version, arch string) (string, error) {
	packageInfo, _ := m.findPackage(packageName, version, arch)
	if packageInfo == nil {
		key := PackageKey(packageName, version, arch)
		return "", fmt.Errorf("package %s not found in cache", key)
	}

	return packageInfo.LocalPath, nil
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
	log.Infof("Downloading and caching package %s-%s-%s", packageName, version, arch)

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

	log.Infof("Successfully cached package %s-%s-%s (compressed: %d bytes, original: %d bytes)",
		packageName, version, arch, compressedSize, originalSize)

	return nil
}

// UploadPackageToNode uploads a cached package to a target node (for backward compatibility)
func (m *PackageManager) UploadPackageToNode(ctx context.Context, packageName, version, arch string, sshRunner interfaces.SSHCommandRunner, nodeName string) error {
	log.Infof("Uploading package %s-%s-%s to node %s", packageName, version, arch, nodeName)

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

	// Create remote directory
	remoteDir := filepath.Dir(packageInfo.GetRemotePath())
	createDirCmd := fmt.Sprintf("sudo mkdir -p %s", remoteDir)
	if _, err := sshRunner.RunCommand(nodeName, createDirCmd); err != nil {
		return fmt.Errorf("failed to create remote directory: %w", err)
	}

	// Check if package already exists on remote node
	checkCmd := fmt.Sprintf("test -f %s && echo 'exists' || echo 'not_exists'", packageInfo.GetRemotePath())
	output, err := sshRunner.RunCommand(nodeName, checkCmd)
	if err != nil {
		return fmt.Errorf("failed to check if package exists on remote node: %w", err)
	}

	if strings.TrimSpace(output) == "exists" {
		log.Infof("Package %s-%s-%s already exists on node %s", packageName, version, arch, nodeName)
		return nil
	}

	// Try to use proper file transfer if available, otherwise fall back to SSH command
	if fileTransfer, ok := sshRunner.(interfaces.SSHRunner); ok {
		log.Infof("Uploading package %s-%s-%s to node %s using SCP protocol", packageName, version, arch, nodeName)
		if err := fileTransfer.UploadFile(nodeName, packageInfo.LocalPath, packageInfo.GetRemotePath()); err != nil {
			return fmt.Errorf("failed to upload package file via SCP: %w", err)
		}
	} else if fileTransfer, ok := sshRunner.(interfaces.SSHFileTransfer); ok {
		log.Infof("Uploading package %s-%s-%s to node %s using file transfer", packageName, version, arch, nodeName)
		if err := fileTransfer.UploadFile(nodeName, packageInfo.LocalPath, packageInfo.GetRemotePath()); err != nil {
			return fmt.Errorf("failed to upload package file: %w", err)
		}
	} else {
		// Fall back to the old base64 method for compatibility
		log.Infof("Uploading package %s-%s-%s to node %s using SSH command (fallback)", packageName, version, arch, nodeName)
		if err := m.uploadFileViaSSH(packageInfo.LocalPath, packageInfo.GetRemotePath(), sshRunner, nodeName); err != nil {
			return fmt.Errorf("failed to upload package file: %w", err)
		}
	}

	log.Infof("Successfully uploaded package %s-%s-%s to node %s", packageName, version, arch, nodeName)
	return nil
}

// uploadFileViaSSH uploads a file to a remote node via SSH using base64 encoding
func (m *PackageManager) uploadFileViaSSH(localPath, remotePath string, sshRunner interfaces.SSHCommandRunner, nodeName string) error {
	// Read local file
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read local file: %w", err)
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(data)

	// Use smaller chunks to avoid command line length limits and shell issues
	chunkSize := 64 * 1024 // 64KB chunks to be safe with command line limits
	totalChunks := (len(encoded) + chunkSize - 1) / chunkSize

	log.Infof("Uploading file %s to %s in %d chunks", localPath, remotePath, totalChunks)

	// First, ensure the remote directory exists
	remoteDir := filepath.Dir(remotePath)
	createDirCmd := fmt.Sprintf("sudo mkdir -p %s", remoteDir)
	if _, err := sshRunner.RunCommand(nodeName, createDirCmd); err != nil {
		return fmt.Errorf("failed to create remote directory %s: %w", remoteDir, err)
	}

	// Create the remote file and upload in chunks
	for i := 0; i < totalChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(encoded) {
			end = len(encoded)
		}

		chunk := encoded[start:end]
		var cmd string

		if i == 0 {
			// First chunk - create file using here-document to avoid quote issues
			cmd = fmt.Sprintf("base64 -d <<'EOF' | sudo tee %s > /dev/null\n%s\nEOF", remotePath, chunk)
		} else {
			// Subsequent chunks - append to file using here-document to avoid quote issues
			cmd = fmt.Sprintf("base64 -d <<'EOF' | sudo tee -a %s > /dev/null\n%s\nEOF", remotePath, chunk)
		}

		if _, err := sshRunner.RunCommand(nodeName, cmd); err != nil {
			return fmt.Errorf("failed to upload chunk %d: %w", i+1, err)
		}

		if totalChunks > 1 {
			log.Infof("Uploaded chunk %d/%d", i+1, totalChunks)
		}
	}

	return nil
}

// CleanupOldPackages removes old cached packages to free up space
func (m *PackageManager) CleanupOldPackages(maxAge time.Duration) error {
	log.Infof("Cleaning up packages older than %v", maxAge)

	cutoff := time.Now().Add(-maxAge)
	var toDelete []int

	for i, packageInfo := range m.index.Packages {
		if packageInfo.LastAccessedAt.Before(cutoff) {
			// Remove the file
			if err := os.Remove(packageInfo.LocalPath); err != nil && !os.IsNotExist(err) {
				log.Errorf("Failed to remove package file %s: %v", packageInfo.LocalPath, err)
			} else {
				key := PackageKey(packageInfo.Name, packageInfo.Version, packageInfo.Architecture)
				log.Infof("Removed old package: %s", key)
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

	log.Infof("Cleanup completed, removed %d packages", len(toDelete))
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
