package cni

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/clusterinfo"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// FlannelInstaller is responsible for installing Flannel CNI
type FlannelInstaller struct {
	SSHClient  *ssh.Client
	MasterNode string
	PodCIDR    string
	Version    string
}

// NewFlannelInstaller creates a Flannel installer
func NewFlannelInstaller(sshClient *ssh.Client, masterNode string) *FlannelInstaller {
	return &FlannelInstaller{
		SSHClient:  sshClient,
		MasterNode: masterNode,
		PodCIDR:    "10.244.0.0/16", // Flannel default Pod CIDR
		Version:    "v0.26.7",       // Flannel version
	}
}

// Install installs Flannel CNI
func (f *FlannelInstaller) Install() error {
	// Ensure br_netfilter module is loaded
	brNetfilterCmd := `
sudo modprobe br_netfilter
echo "br_netfilter" | sudo tee /etc/modules-load.d/br_netfilter.conf
`
	_, err := f.SSHClient.RunCommand(brNetfilterCmd)
	if err != nil {
		return fmt.Errorf("failed to load br_netfilter module: %w", err)
	}

	// Use ClusterInfo to get the cluster's Pod CIDR
	clusterInfo := clusterinfo.NewClusterInfo(f.SSHClient)
	podCIDR, err := clusterInfo.GetPodCIDR()
	if err == nil && podCIDR != "" {
		f.PodCIDR = podCIDR
		log.Infof("Retrieved cluster Pod CIDR: %s", f.PodCIDR)
	}

	// Check if Flannel helm repository has been added
	checkRepoCmd := `helm repo list | grep -q "flannel" && echo "Added" || echo "NotAdded"`
	output, err := f.SSHClient.RunCommand(checkRepoCmd)
	if err != nil {
		return fmt.Errorf("failed to check Flannel Helm repository: %w", err)
	}

	// Add the repository if not added
	if strings.TrimSpace(output) != "Added" {
		log.Info("Adding Flannel Helm repository...")
		_, err = f.SSHClient.RunCommand(`
helm repo add flannel https://flannel-io.github.io/flannel/
helm repo update
`)
		if err != nil {
			return fmt.Errorf("failed to add Flannel Helm repository: %w", err)
		}
	} else {
		log.Info("Flannel Helm repository already exists, skipping addition")
	}

	// Check if kube-flannel namespace already exists
	checkNsCmd := `kubectl get ns kube-flannel -o name 2>/dev/null || echo ""`
	output, err = f.SSHClient.RunCommand(checkNsCmd)
	if err != nil {
		return fmt.Errorf("failed to check kube-flannel namespace: %w", err)
	}

	// Create namespace and set labels (if needed)
	if strings.TrimSpace(output) == "" {
		log.Info("Creating kube-flannel namespace...")
		_, err = f.SSHClient.RunCommand(`
kubectl create ns kube-flannel
kubectl label --overwrite ns kube-flannel pod-security.kubernetes.io/enforce=privileged
`)
		if err != nil {
			return fmt.Errorf("failed to create kube-flannel namespace: %w", err)
		}
	} else {
		// Namespace exists, just set labels
		log.Info("kube-flannel namespace already exists, just updating labels")
		_, err = f.SSHClient.RunCommand(`kubectl label --overwrite ns kube-flannel pod-security.kubernetes.io/enforce=privileged`)
		if err != nil {
			return fmt.Errorf("failed to set kube-flannel namespace labels: %w", err)
		}
	}

	// Check if Flannel is already installed
	checkFlannelCmd := `helm ls -n kube-flannel | grep -q "flannel" && echo "Installed" || echo "NotInstalled"`
	output, err = f.SSHClient.RunCommand(checkFlannelCmd)
	if err != nil {
		return fmt.Errorf("failed to check Flannel installation status: %w", err)
	}

	// Install or upgrade Flannel
	if strings.TrimSpace(output) != "Installed" {
		log.Info("Installing Flannel...")
		helmInstallCmd := fmt.Sprintf(`
helm install flannel --set podCidr="%s" --namespace kube-flannel flannel/flannel
`, f.PodCIDR)
		_, err = f.SSHClient.RunCommand(helmInstallCmd)
		if err != nil {
			return fmt.Errorf("failed to install Flannel with Helm: %w", err)
		}
	} else {
		log.Info("Flannel is already installed, performing upgrade to update configuration...")
		helmUpgradeCmd := fmt.Sprintf(`
helm upgrade flannel --set podCidr="%s" --namespace kube-flannel flannel/flannel
`, f.PodCIDR)
		_, err = f.SSHClient.RunCommand(helmUpgradeCmd)
		if err != nil {
			return fmt.Errorf("failed to upgrade Flannel with Helm: %w", err)
		}
	}

	// Wait for Flannel to be ready
	waitCmd := `
kubectl -n kube-flannel wait --for=condition=ready pod -l app=flannel --timeout=120s
`
	_, err = f.SSHClient.RunCommand(waitCmd)
	if err != nil {
		log.Infof("Warning: Timed out waiting for Flannel to be ready, but will continue: %v", err)
	}

	return nil
}

// SetPodCIDR sets the Pod CIDR
func (f *FlannelInstaller) SetPodCIDR(cidr string) {
	f.PodCIDR = cidr
}

// SetVersion sets the Flannel version
func (f *FlannelInstaller) SetVersion(version string) {
	f.Version = version
}
