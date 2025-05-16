package csi

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/ssh"
)

// RookInstaller 负责安装 Rook-Ceph CSI
type RookInstaller struct {
	SSHClient   *ssh.Client
	MasterNode  string
	RookVersion string
	CephVersion string
}

// NewRookInstaller 创建 Rook 安装器
func NewRookInstaller(sshClient *ssh.Client, masterNode string) *RookInstaller {
	return &RookInstaller{
		SSHClient:   sshClient,
		MasterNode:  masterNode,
		RookVersion: "v1.12.9", // Rook 版本
		CephVersion: "17.2.6",  // Ceph 版本
	}
}

// Install 安装 Rook-Ceph
func (r *RookInstaller) Install() error {
	// 克隆 Rook 仓库到主节点
	cloneCmd := fmt.Sprintf(`
git clone --single-branch --branch %s https://github.com/rook/rook.git
cd rook/deploy/examples
kubectl create -f crds.yaml
kubectl create -f common.yaml
kubectl create -f operator.yaml
kubectl create -f cluster.yaml
`, r.RookVersion)

	_, err := r.SSHClient.RunCommand(cloneCmd)
	if err != nil {
		return fmt.Errorf("安装 Rook-Ceph CSI 失败: %w", err)
	}

	return nil
}
