package api

// AddonType represents the type of addon
type AddonType string

const (
	// AddonTypeCNI CNI addon type
	AddonTypeCNI AddonType = "cni"

	// AddonTypeCSI CSI addon type
	AddonTypeCSI AddonType = "csi"

	// AddonTypeLB LoadBalancer addon type
	AddonTypeLB AddonType = "lb"
)

type Addoner interface {
	// Install installs the addon
	Install() error
	// GetType returns the addon type
	GetType() AddonType
}

type CNIType string

const (
	// CNITypeCilium Cilium CNI type
	CNITypeCilium CNIType = "cilium"

	// CNITypeFlannel Flannel CNI type
	CNITypeFlannel CNIType = "flannel"

	// CNITypeNone No CNI installation
	CNITypeNone CNIType = "none"
)

type CSIType string

const (
	// CSITypeRook Rook CSI type
	CSITypeRook CSIType = "rook-ceph"

	// CSITypeLocalPath LocalPath CSI type
	CSITypeLocalPath CSIType = "local-path-provisioner"

	// CSITypeNone No CSI installation
	CSITypeNone CSIType = "none"
)

type LBType string

const (
	// LBTypeMetalLB MetalLB LB type
	LBTypeMetalLB LBType = "metallb"

	// LBTypeNone No LB installation
	LBTypeNone LBType = "none"
)

type CNIAddoner interface {
	Addoner
	// GetType returns the CNI type
	GetCNIType() CNIType
}

type CSIAddoner interface {
	Addoner
	// GetType returns the CSI type
	GetCSIType() CSIType
}

type LBAddoner interface {
	Addoner
	// GetType returns the LB type
	GetLBType() LBType
}
