package initializer

type ProxyMode string

const (
	ProxyModeIPVS     ProxyMode = "ipvs"
	ProxyModeIptables ProxyMode = "iptables"
	ProxyModeNftables ProxyMode = "nftables"
	ProxyModeNone     ProxyMode = "none"
)

// InitOptions defines environment initialization options
type InitOptions struct {
	DisableSwap       bool      // Whether to disable swap
	ProxyMode         ProxyMode // Whether to enable IPVS mode
	ContainerRuntime  string    // Container runtime, default is containerd
	HelmVersion       string
	ContainerdVersion string
	RuncVersion       string
	CNIPluginsVersion string
	K8SVersion        string
	CriCtlVersion     string // crictl version for CRI debugging
	NerdctlVersion    string // nerdctl version for Docker-compatible CLI
	UsePackageCache   bool   // Whether to use package cache system
	UseImageCache     bool   // Whether to use image cache system
	UpdateSystem      bool   // Whether to update system packages before installation
}

// DefaultInitOptions returns default initialization options
func DefaultInitOptions() InitOptions {
	return InitOptions{
		DisableSwap:       true,              // Default to disable swap
		ProxyMode:         ProxyModeIptables, // Default to not enable IPVS
		ContainerRuntime:  "containerd",      // Default to use containerd
		HelmVersion:       "v3.18.0",
		ContainerdVersion: "2.1.0",
		RuncVersion:       "v1.3.0",
		CNIPluginsVersion: "v1.7.1",
		K8SVersion:        "v1.33.1",
		CriCtlVersion:     "v1.33.0", // Latest stable version of crictl
		NerdctlVersion:    "2.1.2",   // Latest stable version of nerdctl
		UsePackageCache:   false,     // Default to not use package cache
		UseImageCache:     false,     // Default to not use image cache
		UpdateSystem:      false,     // Default to not update system packages
	}
}

func (i *InitOptions) EnableNFTables() bool {
	return i.ProxyMode == ProxyModeNftables
}

func (i *InitOptions) EnableIPTables() bool {
	return i.ProxyMode == ProxyModeIptables
}

func (i *InitOptions) EnableIPVS() bool {
	return i.ProxyMode == ProxyModeIPVS
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
