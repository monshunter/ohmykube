package kube

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/monshunter/ohmykube/pkg/cache"
	"github.com/monshunter/ohmykube/pkg/config/default/kubeadm"
	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
)

// ImageSource represents a source of container images
type ImageSource struct {
	Type    string
	Version string
}

// Manager stores kubeadm configuration
type Manager struct {
	sshRunner         interfaces.SSHRunner
	MasterNode        string
	PodCIDR           string
	ServiceCIDR       string
	ClusterDNS        string
	ProxyMode         string
	KubernetesVersion string
	CustomConfigPath  string // Added: custom configuration pathWill be set to interfaces.ClusterImageManager when needed
}

// NewManager creates a new kubeadm configuration
func NewManager(sshManager interfaces.SSHRunner, k8sVersion string,
	masterNode string, proxyMode string) *Manager {
	return &Manager{
		sshRunner:         sshManager,
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
	// Cache required kubeadm images before initialization
	if err := k.cacheKubeadmImages(k.MasterNode); err != nil {
		log.Warningf("Failed to cache kubeadm images: %v", err)
		// Continue with initialization even if image caching fails
	}

	masterIP := k.sshRunner.Address(k.MasterNode)
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
	if err := k.sshRunner.UploadFile(k.MasterNode, configPath, remoteConfigPath); err != nil {
		return fmt.Errorf("failed to transfer kubeadm configuration file to VM: %w", err)
	}

	// Execute kubeadm init
	initCmd := fmt.Sprintf("sudo kubeadm init --config %s --upload-certs", remoteConfigPath)
	_, err = k.sshRunner.RunCommand(k.MasterNode, initCmd)
	if err != nil {
		return fmt.Errorf("failed to execute kubeadm init: %w", err)
	}
	// Configure kubectl
	configCmd := `mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config`
	_, err = k.sshRunner.RunCommand(k.MasterNode, configCmd)
	if err != nil {
		return fmt.Errorf("failed to configure kubectl: %w", err)
	}

	return nil
}

func (k *Manager) PrintJoinCommand() (string, error) {
	joinCmd := `sudo kubeadm token create --print-join-command`
	output, err := k.sshRunner.RunCommand(k.MasterNode, joinCmd)
	if err != nil {
		return "", fmt.Errorf("failed to get join command: %w", err)
	}
	return output, nil
}

// JoinNode adds a node to the cluster
func (k *Manager) JoinNode(nodeName, joinCommand string) error {
	// Preheat all cluster images to accelerate container startup
	if err := k.cacheClusterImages(nodeName); err != nil {
		log.Warningf("Failed to preheat cluster images for node %s: %v", nodeName, err)
		// Continue with join even if cluster image preheating fails
	}

	if !strings.HasPrefix(joinCommand, "sudo ") {
		joinCommand = "sudo " + joinCommand
	}
	_, err := k.sshRunner.RunCommand(nodeName, joinCommand)
	if err != nil {
		return fmt.Errorf("failed to join node %s to cluster: %w", nodeName, err)
	}
	log.Debugf("Node %s has been successfully joined to cluster!", nodeName)
	return nil

}

// GetKubeconfig gets kubeconfig to local
func (k *Manager) GetKubeconfig() (string, error) {
	// Get kubeconfig content
	output, err := k.sshRunner.RunCommand(k.MasterNode, "cat $HOME/.kube/config")
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

// cacheKubeadmImages caches required kubeadm images before cluster initialization
func (k *Manager) cacheKubeadmImages(nodeName string) error {
	ctx := context.Background()

	// Create image manager with default configuration
	imageManager, err := cache.GetImageManager()
	if err != nil {
		return fmt.Errorf("failed to create image manager: %w", err)
	}

	// Define kubeadm image source for cache
	cacheSource := cache.ImageSource{
		Type:    "kubeadm",
		Version: k.KubernetesVersion,
	}

	// Cache images for node - this will automatically record them via the callback
	return imageManager.EnsureImages(ctx, cacheSource, k.sshRunner, nodeName, k.MasterNode)
}

// cacheClusterImages caches all cluster images to the target node for preheating
func (k *Manager) cacheClusterImages(nodeName string) error {
	// Create image manager with default configuration
	imageManager, err := cache.GetImageManager()
	if err != nil {
		return fmt.Errorf("failed to create image manager: %w", err)
	}

	// Cache all cluster images for the node
	return imageManager.CacheClusterImagesForNodes([]string{nodeName}, k.sshRunner)
}

// DownloadToLocal downloads kubeconfig from remote host to local
// sshClient: SSH client
// clusterName: cluster name, used for naming the downloaded file
// remotePath: remote kubeconfig file path, default is /etc/kubernetes/admin.conf
// Returns local kubeconfig path and error
func (k *Manager) DownloadKubeConfig(clusterName string, remotePath string) (string, error) {
	if remotePath == "" {
		remotePath = "/etc/kubernetes/admin.conf"
	}

	// Get kubeconfig content
	cmd := fmt.Sprintf("cat %s", remotePath)
	kubeconfigContent, err := k.sshRunner.RunCommand(k.MasterNode, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get kubeconfig content: %w", err)
	}

	// Create local kubeconfig file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	kubeDir := filepath.Join(homeDir, ".kube")
	if err := os.MkdirAll(kubeDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create .kube directory: %w", err)
	}

	kubeconfigPath := filepath.Join(kubeDir, clusterName+"-config")
	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644); err != nil {
		return "", fmt.Errorf("failed to save kubeconfig file: %w", err)
	}

	return kubeconfigPath, nil
}
