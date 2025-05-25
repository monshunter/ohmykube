package kube

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/monshunter/ohmykube/pkg/ssh"
)

// DownloadToLocal downloads kubeconfig from remote host to local
// sshClient: SSH client
// clusterName: cluster name, used for naming the downloaded file
// remotePath: remote kubeconfig file path, default is /etc/kubernetes/admin.conf
// Returns local kubeconfig path and error
func DownloadToLocal(sshClient *ssh.Client, clusterName string, remotePath string) (string, error) {
	if remotePath == "" {
		remotePath = "/etc/kubernetes/admin.conf"
	}

	// Get kubeconfig content
	kubeconfigCmd := fmt.Sprintf("cat %s", remotePath)
	kubeconfigContent, err := sshClient.RunCommand(kubeconfigCmd)
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

// GetKubeconfigPath only returns the local path of kubeconfig (without downloading)
func GetKubeconfigPath(clusterName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".kube", clusterName+"-config"), nil
}
