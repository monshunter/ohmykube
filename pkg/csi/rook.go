package csi

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/monshunter/ohmykube/pkg/multipass"
)

// RookInstaller 负责安装 Rook-Ceph CSI
type RookInstaller struct {
	MultipassClient *multipass.Client
	MasterNode      string
	RookVersion     string
	CephVersion     string
}

// NewRookInstaller 创建 Rook 安装器
func NewRookInstaller(mpClient *multipass.Client, masterNode string) *RookInstaller {
	return &RookInstaller{
		MultipassClient: mpClient,
		MasterNode:      masterNode,
		RookVersion:     "v1.12.8", // Rook 版本
		CephVersion:     "17.2.6",  // Ceph 版本
	}
}

// Install 安装 Rook-Ceph
func (r *RookInstaller) Install() error {
	// 克隆 Rook 仓库到主节点
	cloneCmd := fmt.Sprintf(`
if [ ! -d "rook" ]; then
  git clone --single-branch --branch %s https://github.com/rook/rook.git
  cd rook/deploy/examples
  # 替换 Ceph 版本
  sed -i 's|image: quay.io/ceph/ceph:.*|image: quay.io/ceph/ceph:v%s|' cluster.yaml
fi
`, r.RookVersion, r.CephVersion)

	_, err := r.MultipassClient.ExecCommand(r.MasterNode, cloneCmd)
	if err != nil {
		return fmt.Errorf("克隆 Rook 仓库失败: %w", err)
	}

	// 创建 Rook 操作员
	_, err = r.MultipassClient.ExecCommand(r.MasterNode, `
cd rook/deploy/examples
kubectl create -f crds.yaml
kubectl create -f common.yaml
kubectl create -f operator.yaml

# 等待 Rook 操作员就绪
kubectl -n rook-ceph wait --for=condition=ready pod -l app=rook-ceph-operator --timeout=5m
`)
	if err != nil {
		return fmt.Errorf("部署 Rook 操作员失败: %w", err)
	}

	// 为 worker 节点添加标签以支持存储
	_, err = r.MultipassClient.ExecCommand(r.MasterNode, `
for node in $(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o name); do
  kubectl label $node role=storage-node
done
`)
	if err != nil {
		return fmt.Errorf("为节点添加存储标签失败: %w", err)
	}

	// 创建 Ceph 存储集群
	// 为 Ceph 集群创建一个配置文件，只使用 worker 节点
	cephClusterConfig := `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v17.2.6
  dataDirHostPath: /var/lib/rook
  mon:
    count: 1
    allowMultiplePerNode: true
  dashboard:
    enabled: true
    ssl: false
  monitoring:
    enabled: false
  placement:
    all:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: role
              operator: In
              values:
              - storage-node
  storage:
    useAllNodes: false
    useAllDevices: false
    nodes:
      - name: WORKER_NODE_NAME
        config:
          storeType: bluestore
    directories:
      - path: /var/lib/rook/osd0
`

	// 创建临时文件
	tmpfile, err := ioutil.TempFile("", "ceph-cluster-*.yaml")
	if err != nil {
		return fmt.Errorf("创建临时 Ceph 集群配置文件失败: %w", err)
	}
	defer os.Remove(tmpfile.Name())

	// 获取 worker 节点名称
	workerNodeOutput, err := r.MultipassClient.ExecCommand(r.MasterNode, `kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o name | head -1 | cut -d/ -f2`)
	if err != nil {
		return fmt.Errorf("获取 worker 节点名称失败: %w", err)
	}
	workerNodeName := workerNodeOutput
	if workerNodeName == "" {
		workerNodeName = "$(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o name | head -1 | cut -d/ -f2)"
	}

	// 使用实际的 worker 节点名称替换占位符
	cephClusterConfig = fmt.Sprintf(cephClusterConfig, workerNodeName)

	if _, err := tmpfile.Write([]byte(cephClusterConfig)); err != nil {
		return fmt.Errorf("写入 Ceph 集群配置文件失败: %w", err)
	}
	if err := tmpfile.Close(); err != nil {
		return fmt.Errorf("关闭 Ceph 集群配置文件失败: %w", err)
	}

	// 将配置文件传输到虚拟机
	remoteConfigPath := "/tmp/ceph-cluster.yaml"
	if err := r.MultipassClient.TransferFile(tmpfile.Name(), r.MasterNode, remoteConfigPath); err != nil {
		return fmt.Errorf("传输 Ceph 集群配置文件到虚拟机失败: %w", err)
	}

	// 创建 Ceph 集群
	_, err = r.MultipassClient.ExecCommand(r.MasterNode, fmt.Sprintf(`
kubectl create -f %s
`, remoteConfigPath))
	if err != nil {
		return fmt.Errorf("创建 Ceph 集群失败: %w", err)
	}

	// 等待 Ceph 集群就绪 (这可能需要几分钟)
	_, err = r.MultipassClient.ExecCommand(r.MasterNode, `
echo "等待 Ceph 集群就绪 (这可能需要几分钟)..."
kubectl -n rook-ceph wait --for=condition=ready pod -l app=rook-ceph-osd --timeout=10m || true
`)
	if err != nil {
		fmt.Println("警告: Ceph 集群可能仍在启动中，请稍后检查状态")
	}

	// 创建存储类
	storageClassConfig := `apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: rook-ceph
spec:
  failureDomain: host
  replicated:
    size: 1
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-ceph-block
provisioner: rook-ceph.rbd.csi.ceph.com
parameters:
  clusterID: rook-ceph
  pool: replicapool
  imageFormat: "2"
  imageFeatures: layering
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: rook-ceph
  csi.storage.k8s.io/controller-expand-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/controller-expand-secret-namespace: rook-ceph
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-rbd-node
  csi.storage.k8s.io/node-stage-secret-namespace: rook-ceph
  csi.storage.k8s.io/fstype: ext4
allowVolumeExpansion: true
reclaimPolicy: Delete
`

	// 创建临时文件
	storageClassFile, err := ioutil.TempFile("", "storage-class-*.yaml")
	if err != nil {
		return fmt.Errorf("创建临时存储类配置文件失败: %w", err)
	}
	defer os.Remove(storageClassFile.Name())

	if _, err := storageClassFile.Write([]byte(storageClassConfig)); err != nil {
		return fmt.Errorf("写入存储类配置文件失败: %w", err)
	}
	if err := storageClassFile.Close(); err != nil {
		return fmt.Errorf("关闭存储类配置文件失败: %w", err)
	}

	// 将配置文件传输到虚拟机
	remoteStorageClassPath := "/tmp/storage-class.yaml"
	if err := r.MultipassClient.TransferFile(storageClassFile.Name(), r.MasterNode, remoteStorageClassPath); err != nil {
		return fmt.Errorf("传输存储类配置文件到虚拟机失败: %w", err)
	}

	// 创建存储类
	_, err = r.MultipassClient.ExecCommand(r.MasterNode, fmt.Sprintf(`
kubectl create -f %s
`, remoteStorageClassPath))
	if err != nil {
		return fmt.Errorf("创建存储类失败: %w", err)
	}

	// 将 rook-ceph-block 设置为默认存储类
	_, err = r.MultipassClient.ExecCommand(r.MasterNode, `
kubectl patch storageclass rook-ceph-block -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
`)
	if err != nil {
		return fmt.Errorf("将 rook-ceph-block 设置为默认存储类失败: %w", err)
	}

	return nil
}
