package lb

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/default/metallb"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// MetalLBInstaller used for installing and configuring MetalLB load balancer
type MetalLBInstaller struct {
	sshClient *ssh.Client
	masterIP  string
}

// NewMetalLBInstaller creates a new MetalLB installer
func NewMetalLBInstaller(sshClient *ssh.Client, masterIP string) *MetalLBInstaller {
	return &MetalLBInstaller{
		sshClient: sshClient,
		masterIP:  masterIP,
	}
}

// Install installs MetalLB load balancer
func (m *MetalLBInstaller) Install() error {
	// Install MetalLB
	metallbCmd := `
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.9/config/manifests/metallb-native.yaml
`
	_, err := m.sshClient.RunCommand(metallbCmd)
	if err != nil {
		return fmt.Errorf("failed to install MetalLB: %w", err)
	}

	// Wait for MetalLB deployment to complete
	waitCmd := `
kubectl wait --namespace metallb-system --for=condition=ready pod --selector=app=metallb --timeout=90s
`
	_, err = m.sshClient.RunCommand(waitCmd)
	if err != nil {
		return fmt.Errorf("failed to wait for MetalLB deployment to complete: %w", err)
	}

	// Get node IP address range
	ipRange, err := m.getMetalLBAddressRange()
	if err != nil {
		return fmt.Errorf("failed to get MetalLB address range: %w", err)
	}

	// Import configuration template
	configYAML := strings.Replace(metallb.CONFIG_YAML, "# - 192.168.64.200 - 192.168.64.250", fmt.Sprintf("- %s", ipRange), 1)

	// Create configuration file
	configFile := "/tmp/metallb-config.yaml"
	createConfigCmd := fmt.Sprintf("cat <<EOF | tee %s\n%sEOF", configFile, configYAML)
	_, err = m.sshClient.RunCommand(createConfigCmd)
	if err != nil {
		return fmt.Errorf("failed to create MetalLB configuration file: %w", err)
	}

	// Apply configuration
	applyConfigCmd := fmt.Sprintf("kubectl apply -f %s", configFile)
	_, err = m.sshClient.RunCommand(applyConfigCmd)
	if err != nil {
		return fmt.Errorf("failed to apply MetalLB configuration: %w", err)
	}

	return nil
}

// getMetalLBAddressRange gets suitable IP address range for MetalLB
func (m *MetalLBInstaller) getMetalLBAddressRange() (string, error) {
	// Parse IP address
	ipParts := strings.Split(m.masterIP, ".")
	if len(ipParts) != 4 {
		return "", fmt.Errorf("invalid IP address format: %s", m.masterIP)
	}

	// Use a range of IP addresses in the same subnet as LoadBalancer IP pool
	// Example: 192.168.64.100 -> 192.168.64.200-192.168.64.250
	prefix := strings.Join(ipParts[:3], ".")
	startIP := 200
	endIP := 250

	return fmt.Sprintf("%s.%d - %s.%d", prefix, startIP, prefix, endIP), nil
}
