package csi

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/ssh"
)

// LocalPathInstaller 负责安装 local-path-provisioner CSI
type LocalPathInstaller struct {
	SSHClient  *ssh.Client
	MasterNode string
	Version    string
}

// NewLocalPathInstaller 创建 local-path-provisioner 安装器
func NewLocalPathInstaller(sshClient *ssh.Client, masterNode string) *LocalPathInstaller {
	return &LocalPathInstaller{
		SSHClient:  sshClient,
		MasterNode: masterNode,
		Version:    "v0.0.31", // local-path-provisioner 版本
	}
}

// Install 安装 local-path-provisioner
func (l *LocalPathInstaller) Install() error {
	// 安装 local-path-provisioner
	localPathCmd := fmt.Sprintf(`
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/%s/deploy/local-path-storage.yaml
`, l.Version)

	_, err := l.SSHClient.RunCommand(localPathCmd)
	if err != nil {
		return fmt.Errorf("安装 local-path-provisioner 失败: %w", err)
	}

	// 将 local-path 设置为默认存储类
	defaultStorageClassCmd := `
kubectl patch storageclass local-path -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
`
	_, err = l.SSHClient.RunCommand(defaultStorageClassCmd)
	if err != nil {
		return fmt.Errorf("将 local-path 设置为默认存储类失败: %w", err)
	}

	return nil
}
