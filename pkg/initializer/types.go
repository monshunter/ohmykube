package initializer

import (
	"context"

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

// InitOptions defines environment initialization options
type InitOptions struct {
	DisableSwap       bool   // Whether to disable swap
	EnableIPVS        bool   // Whether to enable IPVS mode
	ContainerRuntime  string // Container runtime, default is containerd
	HelmVersion       string
	ContainerdVersion string
	RuncVersion       string
	CNIPluginsVersion string
	K8SVersion        string
	CriCtlVersion     string // crictl version for CRI debugging
	NerdctlVersion    string // nerdctl version for Docker-compatible CLI
}

// DefaultInitOptions returns default initialization options
func DefaultInitOptions() InitOptions {
	return InitOptions{
		DisableSwap:       true,         // Default to disable swap
		EnableIPVS:        false,        // Default to not enable IPVS
		ContainerRuntime:  "containerd", // Default to use containerd
		HelmVersion:       "v3.18.0",
		ContainerdVersion: "2.1.0",
		RuncVersion:       "v1.3.0",
		CNIPluginsVersion: "v1.7.1",
		K8SVersion:        "v1.33.1",
		CriCtlVersion:     "v1.33.0", // Latest stable version of crictl
		NerdctlVersion:    "2.1.2",   // Latest stable version of nerdctl
	}
}

// EnvironmentInitializer is the interface for environment initializers
type EnvironmentInitializer interface {
	// DisableSwap disables swap
	DisableSwap() error

	// EnableIPVS enables IPVS module
	EnableIPVS() error

	// InstallContainerd installs and configures containerd
	InstallContainerd() error

	// InstallK8sComponents installs kubeadm, kubectl, kubelet
	InstallK8sComponents() error

	// Initialize performs all initialization steps
	Initialize() error
}

// NodeInitResult represents the initialization result of a single node
type NodeInitResult struct {
	NodeName string
	Success  bool
	Error    error
}

// BatchInitializer is the interface for batch environment initializers, supporting parallel initialization of multiple nodes
type BatchInitializer interface {
	// Initialize initializes all nodes in parallel
	Initialize() error

	// InitializeWithConcurrencyLimit initializes in parallel with concurrency limit
	InitializeWithConcurrencyLimit(maxConcurrency int) error

	// InitializeWithResults initializes all nodes in parallel and returns detailed results
	InitializeWithResults() []NodeInitResult

	// InitializeWithConcurrencyLimitAndResults initializes with concurrency limit and returns detailed results
	InitializeWithConcurrencyLimitAndResults(maxConcurrency int) []NodeInitResult
}
