package cni

// CNIType represents the supported CNI types
type CNIType string

const (
	// CNITypeCilium Cilium CNI type
	CNITypeCilium CNIType = "cilium"

	// CNITypeFlannel Flannel CNI type
	CNITypeFlannel CNIType = "flannel"

	// CNITypeNone No CNI installation
	CNITypeNone CNIType = "none"
)

// CNIInstaller CNI installer interface
type CNIInstaller interface {
	// Install installs the CNI
	Install() error
}
