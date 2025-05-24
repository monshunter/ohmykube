package cache

import (
	"time"
)

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
