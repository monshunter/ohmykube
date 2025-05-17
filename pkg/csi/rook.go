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
		RookVersion: "v1.17.2", // Rook 版本
		CephVersion: "17.2.6",  // Ceph 版本
	}
}

// Install 安装 Rook-Ceph
func (r *RookInstaller) Install() error {
	// 使用Helm安装Rook-Ceph
	helmInstallCmd := fmt.Sprintf(`
# 添加Rook Helm仓库
helm repo add rook-release https://charts.rook.io/release
helm repo update

# 安装Rook Ceph Operator
helm install --create-namespace --namespace rook-ceph rook-ceph rook-release/rook-ceph

# 等待operator pod就绪
kubectl wait --for=condition=ready pod -l app=rook-ceph-operator -n rook-ceph --timeout=300s

# 安装Rook Ceph Cluster
helm install --create-namespace --namespace rook-ceph rook-ceph-cluster \
   --set operatorNamespace=rook-ceph rook-release/rook-ceph-cluster
`)

	_, err := r.SSHClient.RunCommand(helmInstallCmd)
	if err != nil {
		return fmt.Errorf("安装 Rook-Ceph CSI 失败: %w", err)
	}

	return nil
}
