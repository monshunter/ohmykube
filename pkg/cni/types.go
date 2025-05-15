package cni

// CNIType 表示支持的CNI类型
type CNIType string

const (
	// CNITypeCilium Cilium CNI类型
	CNITypeCilium CNIType = "cilium"

	// CNITypeFlannel Flannel CNI类型
	CNITypeFlannel CNIType = "flannel"

	// CNITypeNone 不安装CNI
	CNITypeNone CNIType = "none"
)

// CNIInstaller CNI安装器接口
type CNIInstaller interface {
	// Install 安装CNI
	Install() error
}
