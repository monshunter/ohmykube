package cni

import (
	"fmt"
	"os"

	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// CiliumInstaller is responsible for installing Cilium CNI
type CiliumInstaller struct {
	SSHClient     *ssh.Client
	MasterNode    string
	Version       string
	APIServerHost string
	APIServerPort string
}

// NewCiliumInstaller creates a Cilium installer
func NewCiliumInstaller(sshClient *ssh.Client, masterNode string, masterIP string) *CiliumInstaller {
	return &CiliumInstaller{
		SSHClient:     sshClient,
		MasterNode:    masterNode,
		Version:       "1.14.5", // Cilium version
		APIServerHost: masterIP, // Use master node IP as API server address
		APIServerPort: "6443",   // Default API server port
	}
}

// Install installs Cilium CNI
func (c *CiliumInstaller) Install() error {
	// Prepare Cilium configuration
	ciliumConfig := `apiVersion: helm.k8s.io/v1
kind: HelmRelease
metadata:
  name: cilium
  namespace: kube-system
spec:
  chart:
    repository: https://helm.cilium.io
    name: cilium
    version: %s
  values:
    ipam:
      mode: kubernetes
    hubble:
      relay:
        enabled: true
      ui:
        enabled: true
    kubeProxyReplacement: true
    k8sServiceHost: %s
    k8sServicePort: %s
`
	ciliumConfig = fmt.Sprintf(ciliumConfig, c.Version, c.APIServerHost, c.APIServerPort)

	// Create temporary file
	tmpfile, err := os.CreateTemp("", "cilium-config-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temporary Cilium config file: %w", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(ciliumConfig)); err != nil {
		return fmt.Errorf("failed to write to Cilium config file: %w", err)
	}
	if err := tmpfile.Close(); err != nil {
		return fmt.Errorf("failed to close Cilium config file: %w", err)
	}

	// Transfer config file to VM
	remoteConfigPath := "/tmp/cilium-config.yaml"
	if err := c.SSHClient.TransferFile(tmpfile.Name(), remoteConfigPath); err != nil {
		return fmt.Errorf("failed to transfer Cilium config file to VM: %w", err)
	}
	// Add Cilium Helm repository
	_, err = c.SSHClient.RunCommand(`
helm repo add cilium https://helm.cilium.io
helm repo update
`)
	if err != nil {
		return fmt.Errorf("failed to add Cilium Helm repository: %w", err)
	}

	// Install Cilium
	log.Info("Installing Cilium...")
	installCmd := fmt.Sprintf(`
helm install cilium cilium/cilium --version %s \
  --namespace kube-system \
  --set ipam.mode=kubernetes \
  --set hubble.relay.enabled=true \
  --set hubble.ui.enabled=true \
  --set kubeProxyReplacement=true \
  --set k8sServiceHost=%s \
  --set k8sServicePort=%s
`, c.Version, c.APIServerHost, c.APIServerPort)

	_, err = c.SSHClient.RunCommand(installCmd)
	if err != nil {
		return fmt.Errorf("failed to install Cilium: %w", err)
	}

	// Wait for Cilium to be ready
	log.Info("Waiting for Cilium to become ready...")
	_, err = c.SSHClient.RunCommand(`
kubectl -n kube-system wait --for=condition=ready pod -l k8s-app=cilium --timeout=5m
`)
	if err != nil {
		return fmt.Errorf("timed out waiting for Cilium to be ready: %w", err)
	}

	log.Info("Cilium installation completed successfully")
	return nil
}
