package initializer

import (
	"os"
)

// SSHCommandRunner defines the interface for executing SSH commands
type SSHCommandRunner interface {
	RunSSHCommand(nodeName string, command string) (string, error)
}

// InitOptions defines environment initialization options
type InitOptions struct {
	DisableSwap       bool   // Whether to disable swap
	EnableIPVS        bool   // Whether to enable IPVS mode
	ContainerRuntime  string // Container runtime, default is containerd
	K8sMirrorURL      string // Kubernetes source URL, default is official source
	HelmVersion       string
	ContainerdVersion string
	RuncVersion       string
	CNIPluginsVersion string
	K8SVersion        string
}

// DefaultInitOptions returns default initialization options
func DefaultInitOptions() InitOptions {
	// Read K8s source URL from environment variable, if not set use the official source
	k8sMirrorURL := os.Getenv("OHMYKUBE_K8S_MIRROR_URL")
	if k8sMirrorURL == "" {
		k8sMirrorURL = "https://pkgs.k8s.io/core:/stable:/v1.33/deb"
	}

	return InitOptions{
		DisableSwap:       true,         // Default to disable swap
		EnableIPVS:        false,        // Default to not enable IPVS
		ContainerRuntime:  "containerd", // Default to use containerd
		K8sMirrorURL:      k8sMirrorURL, // Use source configured by environment variable or default source
		HelmVersion:       "v3.18.0",
		ContainerdVersion: "2.1.0",
		RuncVersion:       "v1.3.0",
		CNIPluginsVersion: "v1.7.1",
		K8SVersion:        "v1.33.1",
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
