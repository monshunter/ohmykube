package kubeconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/monshunter/ohmykube/pkg/ssh"
)

// DownloadToLocal 从远程主机下载kubeconfig到本地
// sshClient: SSH客户端
// clusterName: 集群名称，用于命名下载的文件
// remotePath: 远程kubeconfig文件路径，默认为/etc/kubernetes/admin.conf
// 返回本地kubeconfig路径和错误
func DownloadToLocal(sshClient *ssh.Client, clusterName string, remotePath string) (string, error) {
	if remotePath == "" {
		remotePath = "/etc/kubernetes/admin.conf"
	}

	// 获取kubeconfig内容
	kubeconfigCmd := fmt.Sprintf("cat %s", remotePath)
	kubeconfigContent, err := sshClient.RunCommand(kubeconfigCmd)
	if err != nil {
		return "", fmt.Errorf("获取kubeconfig内容失败: %w", err)
	}

	// 创建本地kubeconfig文件
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户主目录失败: %w", err)
	}

	kubeDir := filepath.Join(homeDir, ".kube")
	if err := os.MkdirAll(kubeDir, 0755); err != nil {
		return "", fmt.Errorf("创建.kube目录失败: %w", err)
	}

	kubeconfigPath := filepath.Join(kubeDir, clusterName+"-config")
	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644); err != nil {
		return "", fmt.Errorf("保存kubeconfig文件失败: %w", err)
	}

	return kubeconfigPath, nil
}

// GetKubeconfigPath 仅返回kubeconfig的本地路径（不下载）
func GetKubeconfigPath(clusterName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户主目录失败: %w", err)
	}

	return filepath.Join(homeDir, ".kube", clusterName+"-config"), nil
}
