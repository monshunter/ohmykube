package kube

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/monshunter/ohmykube/pkg/config/default/kubeadm"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// Manager stores kubeadm configuration
type Manager struct {
	SSHManager        *ssh.SSHManager
	MasterNode        string
	PodCIDR           string
	ServiceCIDR       string
	ClusterDNS        string
	ProxyMode         string
	KubernetesVersion string
	CustomConfigPath  string // Added: custom configuration path
}

// NewManager creates a new kubeadm configuration
func NewManager(sshManager *ssh.SSHManager, k8sVersion string, masterNode string, proxyMode string) *Manager {
	return &Manager{
		SSHManager:        sshManager,
		MasterNode:        masterNode,
		PodCIDR:           "10.244.0.0/16",
		ServiceCIDR:       "10.96.0.0/12",
		ClusterDNS:        "10.96.0.10",
		ProxyMode:         proxyMode,
		KubernetesVersion: k8sVersion,
	}
}

// InitMaster initializes Kubernetes control plane
func (k *Manager) InitMaster() error {
	masterIP := k.SSHManager.GetIP(k.MasterNode)
	// Use new configuration generation system
	configPath, err := kubeadm.GenerateKubeadmConfig(
		k.KubernetesVersion,
		k.CustomConfigPath,
		k.ProxyMode,
		masterIP,
	)
	if err != nil {
		return fmt.Errorf("failed to generate kubeadm configuration: %w", err)
	}
	defer os.Remove(configPath) // Ensure temporary file is deleted

	// Transfer configuration file to the VM
	remoteConfigPath := "/tmp/kubeadm-config.yaml"
	sshClient, exists := k.SSHManager.GetClient(k.MasterNode)
	if !exists {
		return fmt.Errorf("failed to get SSH client for Master node")
	}
	if err := sshClient.TransferFile(configPath, remoteConfigPath); err != nil {
		return fmt.Errorf("failed to transfer kubeadm configuration file to VM: %w", err)
	}

	// Execute kubeadm init
	initCmd := fmt.Sprintf("sudo kubeadm init --config %s --upload-certs", remoteConfigPath)
	_, err = k.SSHManager.RunCommand(k.MasterNode, initCmd)
	if err != nil {
		return fmt.Errorf("failed to execute kubeadm init: %w", err)
	}
	// Configure kubectl
	configCmd := `mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config`
	_, err = k.SSHManager.RunCommand(k.MasterNode, configCmd)
	if err != nil {
		return fmt.Errorf("failed to configure kubectl: %w", err)
	}

	return nil
}

func (k *Manager) PrintJoinCommand() (string, error) {
	joinCmd := `sudo kubeadm token create --print-join-command`
	output, err := k.SSHManager.RunCommand(k.MasterNode, joinCmd)
	if err != nil {
		return "", fmt.Errorf("failed to get join command: %w", err)
	}
	return output, nil
}

// JoinNode adds a node to the cluster
func (k *Manager) JoinNode(nodeName, joinCommand string) error {
	if !strings.HasPrefix(joinCommand, "sudo ") {
		joinCommand = "sudo " + joinCommand
	}
	_, err := k.SSHManager.RunCommand(nodeName, joinCommand)
	if err != nil {
		return fmt.Errorf("failed to join node %s to cluster: %w", nodeName, err)
	}
	log.Infof("Node %s has been successfully joined to cluster!", nodeName)
	return nil

}

// GetKubeconfig gets kubeconfig to local
func (k *Manager) GetKubeconfig() (string, error) {
	// Get kubeconfig content
	output, err := k.SSHManager.RunCommand(k.MasterNode, "cat $HOME/.kube/config")
	if err != nil {
		return "", fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Create local kubeconfig file
	kubeDir := filepath.Join(os.Getenv("HOME"), ".kube")
	if err := os.MkdirAll(kubeDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create .kube directory: %w", err)
	}

	ohmykubeConfig := filepath.Join(kubeDir, "ohmykube-config")
	if err := os.WriteFile(ohmykubeConfig, []byte(output), 0644); err != nil {
		return "", fmt.Errorf("failed to write kubeconfig file: %w", err)
	}

	return ohmykubeConfig, nil
}

// SetCustomConfig sets custom configuration file path
func (k *Manager) SetCustomConfig(configPath string) {
	k.CustomConfigPath = configPath
}
