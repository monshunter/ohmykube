package cluster

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/monshunter/ohmykube/pkg/cni"
	"github.com/monshunter/ohmykube/pkg/csi"
	"github.com/monshunter/ohmykube/pkg/kubeadm"
	"github.com/monshunter/ohmykube/pkg/lb"
	"github.com/monshunter/ohmykube/pkg/multipass"
)

const (
	RoleMaster = "master"
	RoleWorker = "worker"
)

// NodeConfig 保存节点配置
type NodeConfig struct {
	Name string
	ResourceConfig
}

type ResourceConfig struct {
	CPU    int
	Memory int
	Disk   int
}

// ClusterConfig 保存集群配置
type ClusterConfig struct {
	Image      string
	Name       string
	Master     NodeConfig
	Workers    []NodeConfig
	K8sVersion string
	SSHConfig
}

type SSHConfig struct {
	Password      string
	SSHKeyFile    string
	SSHPubKeyFile string
	sshKey        string
	sshPubKey     string
}

func NewSSHConfig(password string, sshKeyFile string, sshPubKeyFile string) (*SSHConfig, error) {
	sshConfig := &SSHConfig{
		Password:      password,
		SSHKeyFile:    sshKeyFile,
		SSHPubKeyFile: sshPubKeyFile,
	}
	err := sshConfig.Init()
	if err != nil {
		return nil, err
	}
	return sshConfig, nil
}

func (c *SSHConfig) Init() error {
	sshKeyContent, err := os.ReadFile(c.SSHKeyFile)
	if err != nil {
		return fmt.Errorf("读取SSH私钥文件失败: %w", err)
	}
	c.sshKey = string(sshKeyContent)
	sshPubKeyContent, err := os.ReadFile(c.SSHPubKeyFile)
	if err != nil {
		return fmt.Errorf("读取SSH公钥文件失败: %w", err)
	}
	c.sshPubKey = string(sshPubKeyContent)
	return nil
}

func (c *SSHConfig) GetSSHKey() string {
	return c.sshKey
}

func (c *SSHConfig) GetSSHPubKey() string {
	return c.sshPubKey
}

func NewClusterConfig(name string, k8sVersion string, workers int, sshConfig *SSHConfig, masterResource ResourceConfig, workerResource ResourceConfig) *ClusterConfig {
	c := &ClusterConfig{
		Name:       name,
		Master:     NodeConfig{},
		Workers:    make([]NodeConfig, workers),
		K8sVersion: k8sVersion,
		SSHConfig:  *sshConfig,
	}
	for i := range workers {
		c.Workers[i].Name = fmt.Sprintf("%s-worker-%d", name, i)
		c.Workers[i].ResourceConfig = workerResource
	}
	c.Master.Name = fmt.Sprintf("%s-master", name)
	c.Master.ResourceConfig = masterResource
	return c
}

func (c *ClusterConfig) GetVMName(role string, index int) string {
	if role == RoleMaster {
		return c.GetMasterVMName(index)
	}
	return c.GetWorkerVMName(index)
}

func (c *ClusterConfig) GetMasterVMName(index int) string {
	return fmt.Sprintf("%smaster-%d", c.Prefix(), index)
}

func (c *ClusterConfig) GetWorkerVMName(index int) string {
	return fmt.Sprintf("%sworker-%d", c.Prefix(), index)
}

func (c *ClusterConfig) Prefix() string {
	return fmt.Sprintf("%s-", c.Name)
}

// Manager 集群管理器
type Manager struct {
	Config          *ClusterConfig
	MultipassClient *multipass.Client
	KubeadmConfig   *kubeadm.KubeadmConfig
}

// NewManager 创建新的集群管理器
func NewManager(config *ClusterConfig) (*Manager, error) {
	mpClient, err := multipass.NewClient(config.Image, config.Password, config.SSHConfig.GetSSHKey(), config.SSHConfig.GetSSHPubKey())
	if err != nil {
		return nil, fmt.Errorf("创建 Multipass 客户端失败: %w", err)
	}

	kubeadmCfg := kubeadm.NewKubeadmConfig(mpClient, config.K8sVersion, config.Master.Name)
	return &Manager{
		Config:          config,
		MultipassClient: mpClient,
		KubeadmConfig:   kubeadmCfg,
	}, nil
}

// CreateCluster 创建一个新的集群
func (m *Manager) CreateCluster() error {
	fmt.Println("开始创建 Kubernetes 集群...")

	// 1. 创建 master 节点
	fmt.Println("创建 Master 节点...")
	err := m.CreateMasterNode()
	if err != nil {
		return fmt.Errorf("创建 Master 节点失败: %w", err)
	}

	// 2. 创建 worker 节点
	fmt.Println("创建 Worker 节点...")
	err = m.CreateWorkerNodes()
	if err != nil {
		return fmt.Errorf("创建 Worker 节点失败: %w", err)
	}

	return nil

	// 3. 配置 master 节点
	fmt.Println("配置 Kubernetes Master 节点...")
	joinCommand, err := m.InitializeMaster()
	if err != nil {
		return fmt.Errorf("初始化 Master 节点失败: %w", err)
	}

	// 4. 配置 worker 节点
	fmt.Println("将 Worker 节点加入集群...")
	err = m.JoinWorkerNodes(joinCommand)
	if err != nil {
		return fmt.Errorf("将 Worker 节点加入集群失败: %w", err)
	}

	// 5. 安装 CNI (Cilium)
	fmt.Println("安装 Cilium CNI...")
	err = m.InstallCNI()
	if err != nil {
		return fmt.Errorf("安装 CNI 失败: %w", err)
	}

	// 6. 安装 CSI (Rook-Ceph)
	fmt.Println("安装 Rook-Ceph CSI...")
	err = m.InstallCSI()
	if err != nil {
		return fmt.Errorf("安装 CSI 失败: %w", err)
	}

	// 7. 安装 MetalLB
	fmt.Println("安装 MetalLB LoadBalancer...")
	err = m.InstallLoadBalancer()
	if err != nil {
		return fmt.Errorf("安装 LoadBalancer 失败: %w", err)
	}

	// 8. 配置 kubeconfig
	fmt.Println("配置本地 kubeconfig...")
	kubeconfigPath, err := m.SetupKubeconfig()
	if err != nil {
		return fmt.Errorf("配置 kubeconfig 失败: %w", err)
	}

	fmt.Printf("\n集群创建成功！可以使用以下命令访问集群:\n")
	fmt.Printf("export KUBECONFIG=%s\n", kubeconfigPath)
	fmt.Printf("kubectl get nodes\n")

	return nil
}

// DeleteCluster 删除集群
func (m *Manager) DeleteCluster() error {
	fmt.Println("正在删除 Kubernetes 集群...")

	// 列出所有虚拟机
	vms, err := m.MultipassClient.ListVMs(m.Config.Name + "-")
	if err != nil {
		return fmt.Errorf("列出虚拟机失败: %w", err)
	}

	// 找到集群相关的虚拟机并删除
	prefix := m.Config.Prefix()
	for _, vm := range vms {
		if strings.HasPrefix(vm, prefix) {
			fmt.Printf("删除节点: %s\n", vm)
			err := m.MultipassClient.DeleteVM(vm)
			if err != nil {
				fmt.Printf("警告: 删除节点 %s 失败: %v\n", vm, err)
			}
		}
	}

	// 删除 kubeconfig
	kubeDir := filepath.Join(os.Getenv("HOME"), ".kube")
	ohmykubeConfig := filepath.Join(kubeDir, m.Config.Name+"-config")
	if _, err := os.Stat(ohmykubeConfig); err == nil {
		os.Remove(ohmykubeConfig)
	}

	fmt.Println("集群删除完成！")
	return nil
}

// CreateMasterNode 创建 master 节点
func (m *Manager) CreateMasterNode() error {
	masterName := m.Config.GetMasterVMName(0)
	cpus := strconv.Itoa(m.Config.Master.CPU)
	memory := strconv.Itoa(m.Config.Master.Memory) + "M"
	disk := strconv.Itoa(m.Config.Master.Disk) + "G"

	return m.MultipassClient.CreateVM(masterName, cpus, memory, disk)
}

// CreateWorkerNodes 创建 worker 节点
func (m *Manager) CreateWorkerNodes() error {
	for i := range m.Config.Workers {
		nodeName := m.Config.GetWorkerVMName(i)
		cpus := strconv.Itoa(m.Config.Workers[i].CPU)
		memory := strconv.Itoa(m.Config.Workers[i].Memory) + "M"
		disk := strconv.Itoa(m.Config.Workers[i].Disk) + "G"

		err := m.MultipassClient.CreateVM(nodeName, cpus, memory, disk)
		if err != nil {
			return fmt.Errorf("创建 Worker 节点 %s 失败: %w", nodeName, err)
		}
	}
	return nil
}

// InitializeMaster 初始化 master 节点
func (m *Manager) InitializeMaster() (string, error) {
	// 等待虚拟机启动完成
	// time.Sleep(5 * time.Second)

	// 在 master 上安装 kubeadm 等组件
	installCmd := `
sudo apt-get update
sudo apt-get install -y apt-transport-https ca-certificates curl git

# 添加 Kubernetes 源
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.28/deb/Release.key | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.28/deb/ /' | sudo tee /etc/apt/sources.list.d/kubernetes.list

sudo apt-get update
sudo apt-get install -y kubelet=1.28.0-1.1 kubeadm=1.28.0-1.1 kubectl=1.28.0-1.1
sudo apt-mark hold kubelet kubeadm kubectl

# 安装 containerd
sudo apt-get install -y containerd

# 配置 containerd 使用 systemd cgroup 驱动
sudo mkdir -p /etc/containerd
containerd config default | sudo tee /etc/containerd/config.toml
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' /etc/containerd/config.toml
sudo systemctl restart containerd

# 禁用 swap
sudo swapoff -a
sudo sed -i '/swap/d' /etc/fstab

# 允许 iptables 查看桥接流量
cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf
overlay
br_netfilter
EOF

sudo modprobe overlay
sudo modprobe br_netfilter

cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF

sudo sysctl --system
`

	_, err := m.MultipassClient.ExecCommand("ohmykube-master", installCmd)
	if err != nil {
		return "", fmt.Errorf("在 Master 节点上安装 Kubernetes 组件失败: %w", err)
	}

	// 在 worker 节点上安装必要组件
	for i := 1; i <= len(m.Config.Workers); i++ {
		nodeName := m.Config.Workers[i-1].Name
		_, err := m.MultipassClient.ExecCommand(nodeName, installCmd)
		if err != nil {
			return "", fmt.Errorf("在 Worker 节点 %s 上安装 Kubernetes 组件失败: %w", nodeName, err)
		}
	}

	// 初始化 master
	return m.KubeadmConfig.InitMaster()
}

// JoinWorkerNodes 将 worker 节点加入集群
func (m *Manager) JoinWorkerNodes(joinCommand string) error {
	for i := 1; i <= len(m.Config.Workers); i++ {
		nodeName := m.Config.Workers[i-1].Name
		err := m.KubeadmConfig.JoinNode(nodeName, joinCommand)
		if err != nil {
			return fmt.Errorf("将 worker 节点 %s 加入集群失败: %w", nodeName, err)
		}
	}
	return nil
}

// InstallCNI 安装 CNI (Cilium)
func (m *Manager) InstallCNI() error {
	ciliumInstaller := cni.NewCiliumInstaller(m.MultipassClient, m.Config.Master.Name)
	return ciliumInstaller.Install()
}

// InstallCSI 安装 CSI (Rook-Ceph)
func (m *Manager) InstallCSI() error {
	rookInstaller := csi.NewRookInstaller(m.MultipassClient, m.Config.Master.Name)
	return rookInstaller.Install()
}

// InstallLoadBalancer 安装 LoadBalancer (MetalLB)
func (m *Manager) InstallLoadBalancer() error {
	metallbInstaller := lb.NewMetalLBInstaller(m.MultipassClient, m.Config.Master.Name)
	return metallbInstaller.Install()
}

// SetupKubeconfig 配置本地 kubeconfig
func (m *Manager) SetupKubeconfig() (string, error) {
	return m.KubeadmConfig.GetKubeconfig()
}

// AddNode 添加新节点到集群
func (m *Manager) AddNode(role string, cpu int, memory int, disk int) error {
	// 确定节点名称
	nodeName := ""
	if role == "master" {
		// 目前只支持单 master
		return fmt.Errorf("目前仅支持单 Master 节点")
	} else {
		// 获取当前 worker 数量
		vms, err := m.MultipassClient.ListVMs(m.Config.Name + "-")
		if err != nil {
			return fmt.Errorf("列出虚拟机失败: %w", err)
		}

		workerCount := 0
		for _, vm := range vms {
			if strings.HasPrefix(vm, "ohmykube-worker-") {
				workerCount++
			}
		}

		nodeName = fmt.Sprintf("ohmykube-worker-%d", workerCount+1)
	}

	// 创建虚拟机
	cpuStr := strconv.Itoa(cpu)
	memoryStr := strconv.Itoa(memory) + "M"
	diskStr := strconv.Itoa(disk) + "G"

	err := m.MultipassClient.CreateVM(nodeName, cpuStr, memoryStr, diskStr)
	if err != nil {
		return fmt.Errorf("创建节点 %s 失败: %w", nodeName, err)
	}

	// 安装必要组件
	installCmd := `
sudo apt-get update
sudo apt-get install -y apt-transport-https ca-certificates curl git

# 添加 Kubernetes 源
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.28/deb/Release.key | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.28/deb/ /' | sudo tee /etc/apt/sources.list.d/kubernetes.list

sudo apt-get update
sudo apt-get install -y kubelet=1.28.0-1.1 kubeadm=1.28.0-1.1 kubectl=1.28.0-1.1
sudo apt-mark hold kubelet kubeadm kubectl

# 安装 containerd
sudo apt-get install -y containerd

# 配置 containerd 使用 systemd cgroup 驱动
sudo mkdir -p /etc/containerd
containerd config default | sudo tee /etc/containerd/config.toml
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' /etc/containerd/config.toml
sudo systemctl restart containerd

# 禁用 swap
sudo swapoff -a
sudo sed -i '/swap/d' /etc/fstab

# 允许 iptables 查看桥接流量
cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf
overlay
br_netfilter
EOF

sudo modprobe overlay
sudo modprobe br_netfilter

cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF

sudo sysctl --system
`

	_, err = m.MultipassClient.ExecCommand(nodeName, installCmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上安装 Kubernetes 组件失败: %w", nodeName, err)
	}

	// 获取 join 命令
	output, err := m.MultipassClient.ExecCommand("ohmykube-master", `
sudo kubeadm token create --print-join-command
`)
	if err != nil {
		return fmt.Errorf("获取 join 命令失败: %w", err)
	}

	// 加入集群
	joinCommand := strings.TrimSpace(output)
	err = m.KubeadmConfig.JoinNode(nodeName, joinCommand)
	if err != nil {
		return fmt.Errorf("节点 %s 加入集群失败: %w", nodeName, err)
	}

	// 如果是 worker，添加存储标签
	if role == "worker" {
		_, err = m.MultipassClient.ExecCommand("ohmykube-master", fmt.Sprintf(`
kubectl label node %s role=storage-node
`, nodeName))
		if err != nil {
			fmt.Printf("警告: 为节点 %s 添加存储标签失败: %v\n", nodeName, err)
		}
	}

	fmt.Printf("节点 %s 已成功添加到集群！\n", nodeName)
	return nil
}

// DeleteNode 从集群中删除节点
func (m *Manager) DeleteNode(nodeName string, force bool) error {
	// 检查节点是否存在
	vms, err := m.MultipassClient.ListVMs(m.Config.Name + "-")
	if err != nil {
		return fmt.Errorf("列出虚拟机失败: %w", err)
	}

	nodeExists := false
	for _, vm := range vms {
		if vm == nodeName {
			nodeExists = true
			break
		}
	}

	if !nodeExists {
		return fmt.Errorf("节点 %s 不存在", nodeName)
	}

	// 如果是 master 节点，拒绝删除
	if nodeName == "ohmykube-master" {
		return fmt.Errorf("不能删除 Master 节点，请先删除整个集群")
	}

	// 从 Kubernetes 中驱逐节点
	if !force {
		fmt.Printf("正在从 Kubernetes 集群中驱逐节点 %s...\n", nodeName)
		_, err = m.MultipassClient.ExecCommand("ohmykube-master", fmt.Sprintf(`
kubectl drain %s --ignore-daemonsets --delete-emptydir-data --force
kubectl delete node %s
`, nodeName, nodeName))
		if err != nil {
			return fmt.Errorf("驱逐节点 %s 失败: %w", nodeName, err)
		}
	}

	// 删除虚拟机
	fmt.Printf("正在删除节点 %s...\n", nodeName)
	err = m.MultipassClient.DeleteVM(nodeName)
	if err != nil {
		return fmt.Errorf("删除节点 %s 失败: %w", nodeName, err)
	}

	fmt.Printf("节点 %s 已成功删除！\n", nodeName)
	return nil
}
