package addons

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/addons/api"
	"github.com/monshunter/ohmykube/pkg/addons/plugins/cni"
	"github.com/monshunter/ohmykube/pkg/addons/plugins/csi"
	"github.com/monshunter/ohmykube/pkg/addons/plugins/lb"
	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
)

// ImageSource represents a source of container images
type ImageSource struct {
	Type    string
	Version string
}

type Manager struct {
	Cluster   *config.Cluster
	sshRunner interfaces.SSHRunner
	CNIType   api.CNIType
	CSIType   api.CSIType
	LBType    api.LBType
}

func NewManager(cluster *config.Cluster, sshRunner interfaces.SSHRunner,
	cniType api.CNIType, csiType api.CSIType, lbType api.LBType) *Manager {
	return &Manager{
		Cluster:   cluster,
		sshRunner: sshRunner,
		CNIType:   cniType,
		CSIType:   csiType,
		LBType:    lbType,
	}
}

// InstallCNI installs CNI
func (m *Manager) InstallCNI() error {
	// Check if CNI is already installed
	if m.Cluster.HasCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusTrue) {
		log.Infof("CNI %s already installed, skipping installation", m.CNIType)
		return nil
	}

	// Set condition to pending
	m.Cluster.SetCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusFalse,
		"Installing", fmt.Sprintf("Installing %s CNI", m.CNIType))

	var err error
	switch m.CNIType {
	case api.CNITypeCilium:
		// Get Master node IP
		ciliumInstaller := cni.NewCiliumInstaller(m.sshRunner, m.Cluster.GetMasterName(), m.Cluster.GetMasterIP())
		err = ciliumInstaller.Install()
		if err != nil {
			m.Cluster.SetCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusFalse,
				"InstallationFailed", fmt.Sprintf("Failed to install Cilium CNI: %v", err))
			return fmt.Errorf("failed to install Cilium CNI: %w", err)
		}

	case api.CNITypeFlannel:
		flannelInstaller := cni.NewFlannelInstaller(m.sshRunner, m.Cluster.GetMasterName())
		err = flannelInstaller.Install()
		if err != nil {
			m.Cluster.SetCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusFalse,
				"InstallationFailed", fmt.Sprintf("Failed to install Flannel CNI: %v", err))
			return fmt.Errorf("failed to install Flannel CNI: %w", err)
		}

	default:
		m.Cluster.SetCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusFalse,
			"UnsupportedType", fmt.Sprintf("Unsupported CNI type: %s", m.CNIType))
		return fmt.Errorf("unsupported CNI type: %s", m.CNIType)
	}

	// Set condition to success
	m.Cluster.SetCondition(config.ConditionTypeCNIInstalled, config.ConditionStatusTrue,
		"Installed", fmt.Sprintf("%s CNI installed successfully", m.CNIType))

	return nil
}

// InstallCSI installs CSI
func (m *Manager) InstallCSI() error {
	// Check if CSI is already installed
	if m.Cluster.HasCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusTrue) {
		log.Infof("%s CSI already installed, skipping installation", m.CSIType)
		return nil
	}

	// Set condition to pending
	m.Cluster.SetCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusFalse,
		"Installing", fmt.Sprintf("Installing %s CSI", m.CSIType))

	var err error
	switch m.CSIType {
	case api.CSITypeRook:
		// Use Rook installer to install Rook-Ceph CSI
		rookInstaller := csi.NewRookInstaller(m.sshRunner, m.Cluster.GetMasterName())
		err = rookInstaller.Install()
		if err != nil {
			m.Cluster.SetCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusFalse,
				"InstallationFailed", fmt.Sprintf("Failed to install Rook-Ceph CSI: %v", err))
			return fmt.Errorf("failed to install Rook-Ceph CSI: %w", err)
		}

	case api.CSITypeLocalPath:
		// Use LocalPath installer to install local-path-provisioner
		localPathInstaller := csi.NewLocalPathInstaller(m.sshRunner, m.Cluster.GetMasterName())
		err = localPathInstaller.Install()
		if err != nil {
			m.Cluster.SetCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusFalse,
				"InstallationFailed", fmt.Sprintf("Failed to install local-path-provisioner: %v", err))
			return fmt.Errorf("failed to install local-path-provisioner: %w", err)
		}

	case api.CSITypeNone:
		m.Cluster.SetCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusTrue,
			"Skipped", "CSI installation was skipped as configured")
		return nil

	default:
		m.Cluster.SetCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusFalse,
			"UnsupportedType", fmt.Sprintf("Unsupported CSI type: %s", m.CSIType))
		return fmt.Errorf("unsupported CSI type: %s", m.CSIType)
	}

	// Set condition to success
	m.Cluster.SetCondition(config.ConditionTypeCSIInstalled, config.ConditionStatusTrue,
		"Installed", fmt.Sprintf("%s CSI installed successfully", m.CSIType))

	return nil
}

// InstallLB installs LoadBalancer (MetalLB)
func (m *Manager) InstallLB() error {
	// Check if LoadBalancer is already installed
	if m.Cluster.HasCondition(config.ConditionTypeLBInstalled, config.ConditionStatusTrue) {
		log.Info("LoadBalancer already installed, skipping installation")
		return nil
	}

	// Set condition to pending
	m.Cluster.SetCondition(config.ConditionTypeLBInstalled, config.ConditionStatusFalse,
		"Installing", "Installing MetalLB LoadBalancer")

	// Use MetalLB installer
	metallbInstaller := lb.NewMetalLBInstaller(m.sshRunner, m.Cluster.GetMasterName(), m.Cluster.GetMasterIP())
	if err := metallbInstaller.Install(); err != nil {
		m.Cluster.SetCondition(config.ConditionTypeLBInstalled, config.ConditionStatusFalse,
			"InstallationFailed", fmt.Sprintf("Failed to install MetalLB: %v", err))
		return fmt.Errorf("failed to install MetalLB: %w", err)
	}

	// Set condition to success
	m.Cluster.SetCondition(config.ConditionTypeLBInstalled, config.ConditionStatusTrue,
		"Installed", "MetalLB LoadBalancer installed successfully")

	return nil
}
