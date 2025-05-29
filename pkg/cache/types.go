package cache

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/monshunter/ohmykube/pkg/interfaces"
)

type ImageSource = interfaces.ImageSource
type ImageCacheManager = interfaces.ImageCacheManager
type ImageDiscovery = interfaces.ImageDiscovery
type ImageRecorder = interfaces.ImageRecorder
type SSHRunner = interfaces.SSHRunner

// PackageType represents the type of package
type PackageType string

const (
	PackageTypeContainerd PackageType = "containerd"
	PackageTypeRunc       PackageType = "runc"
	PackageTypeCNIPlugins PackageType = "cni-plugins"
	PackageTypeKubectl    PackageType = "kubectl"
	PackageTypeKubeadm    PackageType = "kubeadm"
	PackageTypeKubelet    PackageType = "kubelet"
	PackageTypeHelm       PackageType = "helm"
	PackageTypeCrictl     PackageType = "crictl"
	PackageTypeNerdctl    PackageType = "nerdctl"
	PackageTypeImage      PackageType = "image" // Container images
)

// PackageInfo represents information about a cached package
type PackageInfo struct {
	Name           string    `yaml:"name"`
	Version        string    `yaml:"version"`
	Architecture   string    `yaml:"architecture"`
	Type           string    `yaml:"type"`
	DownloadURL    string    `yaml:"download_url"`
	LocalPath      string    `yaml:"local_path"`
	RemotePath     string    `yaml:"remote_path"`
	CompressedSize int64     `yaml:"compressed_size"`
	OriginalSize   int64     `yaml:"original_size"`
	Checksum       string    `yaml:"checksum"`
	CachedAt       time.Time `yaml:"cached_at"`
	LastAccessedAt time.Time `yaml:"last_accessed_at"`
}

// PackageIndex represents the index of all cached packages
type PackageIndex struct {
	Version   string        `yaml:"version"`
	Packages  []PackageInfo `yaml:"packages"`
	UpdatedAt time.Time     `yaml:"updated_at"`
}

// PackageKey generates a unique key for a package
func PackageKey(name, version, arch string) string {
	return name + "-" + version + "-" + arch
}

// GetRemotePath returns the remote path where the package should be stored on target nodes
func (p *PackageInfo) GetRemotePath() string {
	if p.RemotePath != "" {
		return p.RemotePath
	}
	// Default remote path: /usr/local/src/$packageName/$package-$version-$arch.tar.zst
	return "/usr/local/src/" + p.Name + "/" + p.Name + "-" + p.Version + "-" + p.Architecture + ".tar.zst"
}

// FileFormat represents the format of the downloaded file
type FileFormat string

const (
	FileFormatTarGz  FileFormat = "tar.gz"  // .tar.gz files
	FileFormatTgz    FileFormat = "tgz"     // .tgz files
	FileFormatTarBz2 FileFormat = "tar.bz2" // .tar.bz2 files
	FileFormatTarXz  FileFormat = "tar.xz"  // .tar.xz files
	FileFormatTar    FileFormat = "tar"     // .tar files
	FileFormatBinary FileFormat = "binary"  // Plain binary files
)

// PackageDefinition defines how to download and handle different package types
type PackageDefinition struct {
	Name        string
	Type        PackageType
	GetURL      func(version, arch string) string
	GetFilename func(version, arch string) string
	Format      FileFormat // Format of the downloaded file
}

// GetPackageDefinitions returns the definitions for all supported packages
func GetPackageDefinitions() map[string]PackageDefinition {
	return map[string]PackageDefinition{
		"containerd": {
			Name: "containerd",
			Type: PackageTypeContainerd,
			GetURL: func(version, arch string) string {
				return "https://github.com/containerd/containerd/releases/download/v" + version + "/containerd-" + version + "-linux-" + arch + ".tar.gz"
			},
			GetFilename: func(version, arch string) string {
				return "containerd-" + version + "-linux-" + arch + ".tar.gz"
			},
			Format: FileFormatTarGz,
		},
		"runc": {
			Name: "runc",
			Type: PackageTypeRunc,
			GetURL: func(version, arch string) string {
				return "https://github.com/opencontainers/runc/releases/download/" + version + "/runc." + arch
			},
			GetFilename: func(version, arch string) string {
				return "runc." + arch
			},
			Format: FileFormatBinary,
		},
		"cni-plugins": {
			Name: "cni-plugins",
			Type: PackageTypeCNIPlugins,
			GetURL: func(version, arch string) string {
				return "https://github.com/containernetworking/plugins/releases/download/" + version + "/cni-plugins-linux-" + arch + "-" + version + ".tgz"
			},
			GetFilename: func(version, arch string) string {
				return "cni-plugins-linux-" + arch + "-" + version + ".tgz"
			},
			Format: FileFormatTgz,
		},
		"kubectl": {
			Name: "kubectl",
			Type: PackageTypeKubectl,
			GetURL: func(version, arch string) string {
				return "https://dl.k8s.io/release/" + version + "/bin/linux/" + arch + "/kubectl"
			},
			GetFilename: func(version, arch string) string {
				return "kubectl"
			},
			Format: FileFormatBinary,
		},
		"kubeadm": {
			Name: "kubeadm",
			Type: PackageTypeKubeadm,
			GetURL: func(version, arch string) string {
				return "https://dl.k8s.io/release/" + version + "/bin/linux/" + arch + "/kubeadm"
			},
			GetFilename: func(version, arch string) string {
				return "kubeadm"
			},
			Format: FileFormatBinary,
		},
		"kubelet": {
			Name: "kubelet",
			Type: PackageTypeKubelet,
			GetURL: func(version, arch string) string {
				return "https://dl.k8s.io/release/" + version + "/bin/linux/" + arch + "/kubelet"
			},
			GetFilename: func(version, arch string) string {
				return "kubelet"
			},
			Format: FileFormatBinary,
		},
		"helm": {
			Name: "helm",
			Type: PackageTypeHelm,
			GetURL: func(version, arch string) string {
				return "https://get.helm.sh/helm-" + version + "-linux-" + arch + ".tar.gz"
			},
			GetFilename: func(version, arch string) string {
				return "helm-" + version + "-linux-" + arch + ".tar.gz"
			},
			Format: FileFormatTarGz,
		},
		"crictl": {
			Name: "crictl",
			Type: PackageTypeCrictl,
			GetURL: func(version, arch string) string {
				return "https://github.com/kubernetes-sigs/cri-tools/releases/download/" + version + "/crictl-" + version + "-linux-" + arch + ".tar.gz"
			},
			GetFilename: func(version, arch string) string {
				return "crictl-" + version + "-linux-" + arch + ".tar.gz"
			},
			Format: FileFormatTarGz,
		},
		"nerdctl": {
			Name: "nerdctl",
			Type: PackageTypeNerdctl,
			GetURL: func(version, arch string) string {
				return "https://github.com/containerd/nerdctl/releases/download/v" + version + "/nerdctl-" + version + "-linux-" + arch + ".tar.gz"
			},
			GetFilename: func(version, arch string) string {
				return "nerdctl-" + version + "-linux-" + arch + ".tar.gz"
			},
			Format: FileFormatTarGz,
		},
	}
}

// ===== IMAGE CACHE TYPES =====

// ImageReference represents a parsed container image reference
type ImageReference struct {
	Registry string // e.g., docker.io, quay.io, registry.k8s.io
	Project  string // e.g., library, cilium
	Image    string // e.g., nginx, pause
	Tag      string // e.g., latest, v1.0.0
	Digest   string // e.g., sha256:abcdef...
	Original string // Original complete reference
	Arch     string // Architecture: amd64, arm64
}

func (r *ImageReference) Copy() *ImageReference {
	return &ImageReference{
		Registry: r.Registry,
		Project:  r.Project,
		Image:    r.Image,
		Tag:      r.Tag,
		Digest:   r.Digest,
		Original: r.Original,
		Arch:     r.Arch,
	}
}

// ParseImageReference parses an image reference string into its components
func ParseImageReference(ref string, arch string) ImageReference {
	result := ImageReference{
		Original: ref,
		Tag:      "latest", // Default tag
		Arch:     arch,     // Specified architecture
	}

	// Handle digest part
	parts := strings.SplitN(ref, "@", 2)
	if len(parts) > 1 {
		result.Digest = parts[1]
		ref = parts[0]
	}

	// Handle tag part
	parts = strings.SplitN(ref, ":", 2)
	if len(parts) > 1 {
		result.Tag = parts[1]
		ref = parts[0]
	}

	// Handle registry/project/image part
	parts = strings.Split(ref, "/")
	switch len(parts) {
	case 1: // Only image
		result.Registry = "docker.io" // Default registry
		result.Project = "library"    // Default project
		result.Image = parts[0]
	case 2: // project/image
		// Check if first part contains "." or ":", if so it's a registry
		if strings.ContainsAny(parts[0], ".:") {
			result.Registry = parts[0]
			result.Project = ""
			result.Image = parts[1]
		} else {
			result.Registry = "docker.io" // Default registry
			result.Project = parts[0]
			result.Image = parts[1]
		}
	default: // registry/project/image or more levels
		result.Registry = parts[0]
		result.Image = parts[len(parts)-1]
		result.Project = strings.Join(parts[1:len(parts)-1], "/")
	}

	return result
}

// String returns the full image reference
func (r ImageReference) String() string {
	var parts []string

	// Build basic path
	if r.Registry != "" && r.Registry != "docker.io" {
		parts = append(parts, r.Registry)
	}

	if r.Project != "" && r.Project != "library" {
		parts = append(parts, r.Project)
	}

	parts = append(parts, r.Image)

	// Combine into full path
	ref := strings.Join(parts, "/")

	// Add tag
	if r.Tag != "" {
		ref = ref + ":" + r.Tag
	}

	// Add digest
	if r.Digest != "" {
		ref = ref + "@" + r.Digest
	}

	return ref
}

// NormalizedName returns a normalized name suitable for file paths
func (r ImageReference) NormalizedName() string {
	// Replace problematic characters in filenames
	name := r.String()
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, "@", "_")

	// Add architecture information
	return fmt.Sprintf("%s_%s", name, r.Arch)
}

// CacheKey returns a unique key for this image reference
func (r ImageReference) CacheKey() string {
	// If there's a digest, use digest as unique identifier
	if r.Digest != "" {
		return fmt.Sprintf("%s/%s/%s@%s_%s",
			r.Registry, r.Project, r.Image, r.Digest, r.Arch)
	}

	// Otherwise use tag
	return fmt.Sprintf("%s/%s/%s:%s_%s",
		r.Registry, r.Project, r.Image, r.Tag, r.Arch)
}

// ImageInfo stores information about a cached image
type ImageInfo struct {
	Name             string         `yaml:"name"`
	Reference        ImageReference `yaml:"reference"`
	LocalPath        string         `yaml:"localpath"`
	Size             int64          `yaml:"size"`
	LastAccessed     time.Time      `yaml:"lastaccessed"`
	LastUpdated      time.Time      `yaml:"lastupdated"`
	OriginalSize     int64          `yaml:"originalsize,omitempty"`
	CompressionRatio float64        `yaml:"compressionratio,omitempty"`
	Architectures    []string       `yaml:"architectures,omitempty"` // Supported architectures
}

func (i *ImageInfo) Copy() *ImageInfo {
	return &ImageInfo{
		Name:             i.Name,
		Reference:        *i.Reference.Copy(),
		LocalPath:        i.LocalPath,
		Size:             i.Size,
		LastAccessed:     i.LastAccessed,
		LastUpdated:      i.LastUpdated,
		OriginalSize:     i.OriginalSize,
		CompressionRatio: i.CompressionRatio,
		Architectures:    i.Architectures,
	}
}

// ImageIndex represents the index of all cached images
type ImageIndex struct {
	Version   string      `yaml:"version"`
	Images    []ImageInfo `yaml:"images"`
	UpdatedAt time.Time   `yaml:"updated_at"`
	lock      sync.RWMutex
}

// findImageByName finds an image in the array by name (cache key)
func (m *ImageIndex) findImageByName(name string) (*ImageInfo, int) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	for i, img := range m.Images {
		if img.Name == name {
			return &img, i
		}
	}
	return nil, -1
}

// removeImageByName removes an image from the array by name (cache key)
func (m *ImageIndex) removeImageByName(name string) bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	for i, img := range m.Images {
		if img.Name == name {
			// Remove by swapping with last element and truncating
			m.Images[i] = m.Images[len(m.Images)-1]
			m.Images = m.Images[:len(m.Images)-1]
			return true
		}
	}
	return false
}

// updateOrAddImage updates an existing image or adds a new one to the array
func (m *ImageIndex) updateOrAddImage(name string, imageInfo ImageInfo) {
	m.lock.Lock()
	defer m.lock.Unlock()
	imageInfo.Name = name
	for i, img := range m.Images {
		if img.Name == name {
			// Update existing
			m.Images[i] = imageInfo
			return
		}
	}
	// Add new
	m.Images = append(m.Images, imageInfo)
}

func (m *ImageIndex) Copy() *ImageIndex {
	m.lock.RLock()
	defer m.lock.RUnlock()
	newIndex := &ImageIndex{
		Version:   m.Version,
		UpdatedAt: m.UpdatedAt,
	}
	newIndex.Images = make([]ImageInfo, len(m.Images))
	for i, img := range m.Images {
		newIndex.Images[i] = *img.Copy()
	}
	return newIndex
}

// ImageManagementStrategy defines where and how images should be managed
type ImageManagementStrategy int

const (
	// ImageManagementAuto automatically detects the best strategy based on available tools
	ImageManagementAuto ImageManagementStrategy = iota

	// ImageManagementLocal prefers local machine operations (requires nerdctl locally)
	ImageManagementLocal

	// ImageManagementController prefers controller node operations (uses tools on controller)
	ImageManagementController

	// ImageManagementTarget operates directly on target nodes (slowest but most compatible)
	ImageManagementTarget
)

// String returns the string representation of the strategy
func (s ImageManagementStrategy) String() string {
	switch s {
	case ImageManagementAuto:
		return "auto"
	case ImageManagementLocal:
		return "local"
	case ImageManagementController:
		return "controller"
	case ImageManagementTarget:
		return "target"
	default:
		return "unknown"
	}
}

// ImageManagementConfig configures the image management strategy
type ImageManagementConfig struct {
	Strategy        ImageManagementStrategy   // Primary strategy
	FallbackOrder   []ImageManagementStrategy // Ordered list of fallback strategies
	LocalToolsCheck bool                      // Whether to check for local tools availability
	ForceStrategy   bool                      // Whether to force the strategy without fallbacks
}

// DefaultImageManagementConfig returns the default image management configuration
func DefaultImageManagementConfig() ImageManagementConfig {
	return ImageManagementConfig{
		Strategy:        ImageManagementAuto,
		FallbackOrder:   []ImageManagementStrategy{ImageManagementLocal, ImageManagementController, ImageManagementTarget},
		LocalToolsCheck: true,
		ForceStrategy:   false,
	}
}

// LocalPreferredImageManagementConfig returns a configuration that prefers local operations
func LocalPreferredImageManagementConfig() ImageManagementConfig {
	return ImageManagementConfig{
		Strategy:        ImageManagementLocal,
		FallbackOrder:   []ImageManagementStrategy{ImageManagementController, ImageManagementTarget},
		LocalToolsCheck: true,
		ForceStrategy:   false,
	}
}

// ControllerOnlyImageManagementConfig returns a configuration that only uses controller node
func ControllerOnlyImageManagementConfig() ImageManagementConfig {
	return ImageManagementConfig{
		Strategy:        ImageManagementController,
		FallbackOrder:   []ImageManagementStrategy{},
		LocalToolsCheck: false,
		ForceStrategy:   true,
	}
}

// ToolAvailability tracks which tools are available where
type ToolAvailability struct {
	LocalNerdctl bool // nerdctl available locally
	LocalHelm    bool // helm available locally
	LocalKubeadm bool // kubeadm available locally
	LocalCurl    bool // curl available locally

	ControllerNerdctl bool // nerdctl available on controller
	ControllerHelm    bool // helm available on controller
	ControllerKubeadm bool // kubeadm available on controller
	ControllerCurl    bool // curl available on controller
}
