package manager

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/cni"
	"github.com/monshunter/ohmykube/pkg/default/metallb"
	"github.com/monshunter/ohmykube/pkg/environment"
	"github.com/monshunter/ohmykube/pkg/kubeadm"
	"github.com/monshunter/ohmykube/pkg/kubeconfig"
	"github.com/monshunter/ohmykube/pkg/multipass"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// Manager 集群管理器
type Manager struct {
	Config             *cluster.Config
	MultipassClient    *multipass.Client
	KubeadmConfig      *kubeadm.KubeadmConfig
	SSHManager         *ssh.SSHManager
	Cluster            *cluster.Cluster
	InitOptions        environment.InitOptions
	CNIType            string // 使用的CNI类型，默认为flannel
	CSIType            string // 使用的CSI类型，默认为local-path-provisioner
	DownloadKubeconfig bool   // 是否将kubeconfig下载到本地
}

// NewManager 创建新的集群管理器
func NewManager(config *cluster.Config, sshConfig *ssh.SSHConfig, cluster *cluster.Cluster) (*Manager, error) {
	mpClient, err := multipass.NewClient(config.Image, sshConfig.Password, sshConfig.GetSSHKey(), sshConfig.GetSSHPubKey())
	if err != nil {
		return nil, fmt.Errorf("创建 Multipass 客户端失败: %w", err)
	}

	// 创建集群信息对象
	if cluster == nil {
		cluster = clusterFromConfig(config)
	}

	manager := &Manager{
		Config:             config,
		MultipassClient:    mpClient,
		SSHManager:         ssh.NewSSHManager(cluster, sshConfig),
		Cluster:            cluster,
		InitOptions:        environment.DefaultInitOptions(),
		CNIType:            "flannel",                // 默认使用flannel
		CSIType:            "local-path-provisioner", // 默认使用local-path-provisioner
		DownloadKubeconfig: true,                     // 默认下载kubeconfig
	}

	// 稍后在需要使用时创建SSH客户端和KubeadmConfig
	// KubeadmConfig将在初始化Master节点时设置

	return manager, nil
}

func clusterFromConfig(config *cluster.Config) *cluster.Cluster {

	var master cluster.NodeInfo
	var workers []cluster.NodeInfo
	master = cluster.NodeInfo{
		Name:    config.Master.Name,
		Role:    cluster.RoleMaster,
		CPU:     config.Master.CPU,
		Memory:  config.Master.Memory,
		Disk:    config.Master.Disk,
		SSHUser: "root",
		SSHPort: "22",
	}

	for _, worker := range config.Workers {
		workers = append(workers, cluster.NodeInfo{
			Name:    worker.Name,
			Role:    cluster.RoleWorker,
			CPU:     worker.CPU,
			Memory:  worker.Memory,
			Disk:    worker.Disk,
			SSHUser: "root",
			SSHPort: "22",
		})
	}

	return cluster.NewCluster(config.Name, config.K8sVersion, master, workers)
}

// SetInitOptions 设置环境初始化选项
func (m *Manager) SetInitOptions(options environment.InitOptions) {
	m.InitOptions = options
}

// GetNodeIP 获取节点IP地址
func (m *Manager) GetNodeIP(nodeName string) (string, error) {
	// 重试几次，因为节点可能需要一些时间才能完全启动
	var err error
	maxRetries := 5
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		// 使用multipass info命令获取节点信息
		cmd := exec.Command("multipass", "info", nodeName, "--format", "json")
		output, err := cmd.Output()
		if err == nil {
			// 解析JSON输出
			var info struct {
				Info map[string]struct {
					Ipv4 []string `json:"ipv4"`
				} `json:"info"`
			}

			if err := json.Unmarshal(output, &info); err != nil {
				return "", fmt.Errorf("解析节点信息失败: %w", err)
			}

			nodeInfo, ok := info.Info[nodeName]
			if !ok || len(nodeInfo.Ipv4) == 0 {
				// 如果此次尝试未找到节点信息，继续重试
				time.Sleep(retryDelay)
				continue
			}

			return nodeInfo.Ipv4[0], nil
		}

		// 如果失败，等待一段时间后重试
		fmt.Printf("获取节点 %s 的IP地址失败，正在重试 (%d/%d)...\n", nodeName, i+1, maxRetries)
		time.Sleep(retryDelay)
	}

	return "", fmt.Errorf("获取节点 %s 的IP地址失败，已重试 %d 次: %w", nodeName, maxRetries, err)
}

// WaitForSSHReady 等待节点SSH服务就绪
func (m *Manager) WaitForSSHReady(ip string, port string, maxRetries int) error {
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, port), 5*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}

		fmt.Printf("等待SSH服务就绪 (%s:%s)，正在重试 (%d/%d)...\n", ip, port, i+1, maxRetries)
		time.Sleep(retryDelay)
	}

	return fmt.Errorf("等待SSH服务就绪超时 (%s:%s)", ip, port)
}

func (m *Manager) CloseSSHClient() {
	m.SSHManager.CloseAllClients()
}

func (m *Manager) AddNodeInfo(nodeName string, cpu int, memory int, disk int) error {
	extraInfo, err := m.GetExtraInfoFromRemote(nodeName)
	if err != nil {
		return fmt.Errorf("获取节点 %s 信息失败: %w", nodeName, err)
	}
	nodeInfo := cluster.NewNodeInfo(nodeName, cluster.RoleWorker, cpu, memory, disk)
	nodeInfo.ExtraInfo = extraInfo
	m.Cluster.AddNode(nodeInfo)
	return nil
}

// UpdateClusterInfo 更新集群信息
func (m *Manager) UpdateClusterInfo() error {
	extraInfomations := make([]cluster.NodeExtraInfo, 0, 1+len(m.Cluster.Workers))
	masterInfo, err := m.GetExtraInfoFromRemote(m.Cluster.Master.Name)
	if err != nil {
		return fmt.Errorf("获取Master节点信息失败: %w", err)
	}
	extraInfomations = append(extraInfomations, masterInfo)
	for i := range m.Cluster.Workers {
		workerInfo, err := m.GetExtraInfoFromRemote(m.Cluster.Workers[i].Name)
		if err != nil {
			return fmt.Errorf("获取Worker节点信息失败: %w", err)
		}
		extraInfomations = append(extraInfomations, workerInfo)
	}
	m.Cluster.UpdateWithExtraInfo(extraInfomations)
	// 保存集群信息到本地文件
	return cluster.SaveClusterInfomation(m.Cluster)
}

func (m *Manager) GetExtraInfoFromRemote(nodeName string) (info cluster.NodeExtraInfo, err error) {
	// 获取IP信息
	ip, err := m.GetNodeIP(nodeName)
	if err != nil {
		return info, fmt.Errorf("获取节点 %s 的IP失败: %w", nodeName, err)
	}

	// 填充基本信息
	info.Name = nodeName
	info.IP = ip
	info.Status = cluster.NodeStatusRunning
	sshClient, err := m.SSHManager.CreateClient(nodeName, ip)
	if err != nil {
		return info, fmt.Errorf("创建SSH客户端失败: %w", err)
	}
	// 获取主机名
	hostnameCmd := "hostname"
	hostnameOutput, err := sshClient.RunCommand(hostnameCmd)
	if err == nil {
		info.Hostname = strings.TrimSpace(hostnameOutput)
	} else {
		fmt.Printf("警告: 获取节点 %s 的主机名失败: %v\n", nodeName, err)
		return info, fmt.Errorf("获取节点 %s 的主机名失败: %w", nodeName, err)
	}

	// 获取操作系统发行版信息
	releaseCmd := `cat /etc/os-release | grep PRETTY_NAME | cut -d'"' -f2`
	releaseOutput, err := sshClient.RunCommand(releaseCmd)
	if err == nil {
		info.Release = strings.TrimSpace(releaseOutput)
	} else {
		fmt.Printf("警告: 获取节点 %s 的发行版信息失败: %v\n", nodeName, err)
	}

	// 获取内核版本
	kernelCmd := "uname -r"
	kernelOutput, err := sshClient.RunCommand(kernelCmd)
	if err == nil {
		info.Kernel = strings.TrimSpace(kernelOutput)
	} else {
		fmt.Printf("警告: 获取节点 %s 的内核版本失败: %v\n", nodeName, err)
	}

	// 获取系统架构
	archCmd := "uname -m"
	archOutput, err := sshClient.RunCommand(archCmd)
	if err == nil {
		info.Arch = strings.TrimSpace(archOutput)
	} else {
		fmt.Printf("警告: 获取节点 %s 的系统架构失败: %v\n", nodeName, err)
	}

	// 获取操作系统类型
	osCmd := `uname -o`
	osOutput, err := sshClient.RunCommand(osCmd)
	if err == nil {
		info.OS = strings.TrimSpace(osOutput)
	} else {
		fmt.Printf("警告: 获取节点 %s 的操作系统类型失败: %v\n", nodeName, err)
	}

	return info, nil
}

// RunSSHCommand 通过SSH在节点上执行命令
func (m *Manager) RunSSHCommand(nodeName, command string) (string, error) {
	return m.SSHManager.RunCommand(nodeName, command)
}

// SetCNIType 设置CNI类型
func (m *Manager) SetCNIType(cniType string) {
	m.CNIType = cniType
}

// SetCSIType 设置CSI类型
func (m *Manager) SetCSIType(csiType string) {
	m.CSIType = csiType
}

// SetDownloadKubeconfig 设置是否下载kubeconfig到本地
func (m *Manager) SetDownloadKubeconfig(download bool) {
	m.DownloadKubeconfig = download
}

// SetupKubeconfig 配置本地 kubeconfig
func (m *Manager) SetupKubeconfig() (string, error) {
	// 获取Master节点的SSH客户端
	sshClient, exists := m.SSHManager.GetClient(m.Config.Master.Name)
	if !exists {
		return "", fmt.Errorf("获取Master节点SSH客户端失败")
	}

	// 使用统一的kubeconfig下载逻辑
	return kubeconfig.DownloadToLocal(sshClient, m.Config.Name, "")
}

// CreateCluster 创建一个新的集群
func (m *Manager) CreateCluster() error {
	fmt.Println("开始创建 Kubernetes 集群...")

	// 1. 创建所有节点（并行）
	fmt.Println("创建集群节点...")
	err := m.CreateClusterNodes()
	if err != nil {
		return fmt.Errorf("创建集群节点失败: %w", err)
	}

	// 2. 更新集群信息
	fmt.Println("获取集群节点信息...")
	err = m.UpdateClusterInfo()
	if err != nil {
		return fmt.Errorf("更新集群信息失败: %w", err)
	}

	fmt.Println("使用root用户通过SSH连接节点...")

	// 3. 初始化环境
	err = m.InitializeEnvironment()
	if err != nil {
		return fmt.Errorf("初始化环境失败: %w", err)
	}
	// 4. 配置 master 节点
	fmt.Println("配置 Kubernetes Master 节点...")
	joinCommand, err := m.InitializeMaster()
	if err != nil {
		return fmt.Errorf("初始化 Master 节点失败: %w", err)
	}

	// 5. 配置 worker 节点
	fmt.Println("将 Worker 节点加入集群...")
	err = m.JoinWorkerNodes(joinCommand)
	if err != nil {
		return fmt.Errorf("将 Worker 节点加入集群失败: %w", err)
	}

	// 6. 安装 CNI
	if m.CNIType != "none" {
		fmt.Printf("安装 %s CNI...\n", m.CNIType)
		err = m.InstallCNI()
		if err != nil {
			return fmt.Errorf("安装 CNI 失败: %w", err)
		}
	} else {
		fmt.Println("跳过CNI安装...")
	}

	// 7. 安装 CSI
	if m.CSIType != "none" {
		fmt.Printf("安装 %s CSI...\n", m.CSIType)
		err = m.InstallCSI()
		if err != nil {
			return fmt.Errorf("安装 CSI 失败: %w", err)
		}
	} else {
		fmt.Println("跳过CSI安装...")
	}

	// 8. 安装 MetalLB
	fmt.Println("安装 MetalLB LoadBalancer...")
	err = m.InstallLoadBalancer()
	if err != nil {
		return fmt.Errorf("安装 LoadBalancer 失败: %w", err)
	}

	// 9. 下载 kubeconfig 到本地 (可选步骤)
	var kubeconfigPath string
	if m.DownloadKubeconfig {
		fmt.Println("下载 kubeconfig 到本地...")
		kubeconfigPath, err = m.SetupKubeconfig()
		if err != nil {
			return fmt.Errorf("下载 kubeconfig 到本地失败: %w", err)
		}
	} else {
		// 只获取路径但不下载文件
		kubeconfigPath, err = kubeconfig.GetKubeconfigPath(m.Config.Name)
		if err != nil {
			return fmt.Errorf("获取 kubeconfig 路径失败: %w", err)
		}
		fmt.Println("跳过 kubeconfig 下载...")
	}

	fmt.Printf("\n集群创建成功！可以使用以下命令访问集群:\n")
	fmt.Printf("export KUBECONFIG=%s\n", kubeconfigPath)
	fmt.Printf("kubectl get nodes\n")

	return nil
}

// CreateClusterNodes 并行创建集群所有节点（master和worker）
func (m *Manager) CreateClusterNodes() error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(m.Config.Workers)+1) // 为所有节点（包括master）创建错误通道

	// 创建一个包含所有节点配置的切片
	allNodes := make([]struct {
		name   string
		cpu    string
		memory string
		disk   string
	}, 0, len(m.Config.Workers)+1)

	// 添加master节点配置
	allNodes = append(allNodes, struct {
		name   string
		cpu    string
		memory string
		disk   string
	}{
		name:   m.Config.Master.Name,
		cpu:    strconv.Itoa(m.Config.Master.CPU),
		memory: strconv.Itoa(m.Config.Master.Memory) + "M",
		disk:   strconv.Itoa(m.Config.Master.Disk) + "G",
	})

	// 添加worker节点配置
	for i := range m.Config.Workers {
		allNodes = append(allNodes, struct {
			name   string
			cpu    string
			memory string
			disk   string
		}{
			name:   m.Config.Workers[i].Name,
			cpu:    strconv.Itoa(m.Config.Workers[i].CPU),
			memory: strconv.Itoa(m.Config.Workers[i].Memory) + "M",
			disk:   strconv.Itoa(m.Config.Workers[i].Disk) + "G",
		})
	}

	// 并行创建所有节点
	for _, node := range allNodes {
		wg.Add(1)
		go func(nodeName, cpus, memory, disk string) {
			defer wg.Done()
			fmt.Printf("创建节点: %s\n", nodeName)
			if err := m.MultipassClient.CreateVM(nodeName, cpus, memory, disk); err != nil {
				errChan <- fmt.Errorf("创建节点 %s 失败: %w", nodeName, err)
			}
		}(node.name, node.cpu, node.memory, node.disk)
	}

	// 等待所有节点创建完成
	wg.Wait()
	close(errChan)

	// 检查是否有错误发生
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// CreateMasterNode 创建 master 节点
func (m *Manager) CreateMasterNode() error {
	masterName := m.Config.Master.Name
	cpus := strconv.Itoa(m.Config.Master.CPU)
	memory := strconv.Itoa(m.Config.Master.Memory) + "M"
	disk := strconv.Itoa(m.Config.Master.Disk) + "G"

	return m.MultipassClient.CreateVM(masterName, cpus, memory, disk)
}

// CreateWorkerNodes 创建 worker 节点
func (m *Manager) CreateWorkerNodes() error {
	for i := range m.Config.Workers {
		nodeName := m.Config.Workers[i].Name
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

func (m *Manager) InitializeEnvironment() error {
	// 将所有节点名称收集到一个切片中
	nodeNames := []string{m.Config.Master.Name}
	for _, worker := range m.Config.Workers {
		nodeNames = append(nodeNames, worker.Name)
	}

	// 使用并行批量初始化器，支持并行初始化多个节点
	batchInitializer := environment.NewParallelBatchInitializerWithOptions(m, nodeNames, m.InitOptions)

	// 使用并发限制执行初始化（限制为2个节点同时初始化，避免apt锁争用）
	if err := batchInitializer.InitializeWithConcurrencyLimit(3); err != nil {
		return fmt.Errorf("初始化环境失败: %w", err)
	}

	return nil
}

// InitializeMaster 初始化 master 节点
func (m *Manager) InitializeMaster() (string, error) {
	// 为master节点创建SSH客户端
	sshClient, exists := m.SSHManager.GetClient(m.Config.Master.Name)
	if !exists {
		return "", fmt.Errorf("获取Master节点SSH客户端失败")
	}

	// 创建KubeadmConfig
	m.KubeadmConfig = kubeadm.NewKubeadmConfig(sshClient, m.Config.K8sVersion, m.Config.Master.Name)

	// 使用新的配置系统生成配置文件并初始化Master
	err := m.KubeadmConfig.InitMaster()
	if err != nil {
		return "", fmt.Errorf("初始化 Master 节点失败: %w", err)
	}
	return m.joinCommand()
}

func (m *Manager) joinCommand() (string, error) {
	joinCmd := `sudo kubeadm token create --print-join-command`
	output, err := m.RunSSHCommand(m.Config.Master.Name, joinCmd)
	if err != nil {
		return "", fmt.Errorf("获取集群加入命令失败: %w", err)
	}
	return output, nil
}

// JoinWorkerNodes 将 worker 节点加入集群
func (m *Manager) JoinWorkerNodes(joinCommand string) error {
	for i := range m.Config.Workers {
		err := m.JoinWorkerNode(m.Config.Workers[i].Name, joinCommand)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) JoinWorkerNode(nodeName, joinCommand string) error {
	_, err := m.RunSSHCommand(nodeName, joinCommand)
	if err != nil {
		return fmt.Errorf("将节点 %s 加入集群失败: %w", nodeName, err)
	}
	fmt.Printf("节点 %s 已成功加入集群！\n", nodeName)
	return nil
}

// InstallCNI 安装 CNI
func (m *Manager) InstallCNI() error {
	// 确保master节点的SSH客户端已创建
	sshClient, exists := m.SSHManager.GetClient(m.Config.Master.Name)
	if !exists {
		return fmt.Errorf("获取Master节点SSH客户端失败")
	}

	switch m.CNIType {
	case "cilium":
		ciliumInstaller := cni.NewCiliumInstaller(sshClient, m.Config.Master.Name)
		err := ciliumInstaller.Install()
		if err != nil {
			return fmt.Errorf("安装 Cilium CNI 失败: %w", err)
		}

	case "flannel":
		flannelInstaller := cni.NewFlannelInstaller(sshClient, m.Config.Master.Name)
		err := flannelInstaller.Install()
		if err != nil {
			return fmt.Errorf("安装 Flannel CNI 失败: %w", err)
		}

	default:
		return fmt.Errorf("不支持的 CNI 类型: %s", m.CNIType)
	}

	return nil
}

// InstallCSI 安装 CSI
func (m *Manager) InstallCSI() error {

	switch m.CSIType {
	case "rook-ceph":
		// 安装 Rook-Ceph CSI
		rookCmd := `
git clone --single-branch --branch v1.12.9 https://github.com/rook/rook.git
cd rook/deploy/examples
kubectl create -f crds.yaml
kubectl create -f common.yaml
kubectl create -f operator.yaml
kubectl create -f cluster.yaml
`
		_, err := m.RunSSHCommand(m.Config.Master.Name, rookCmd)
		if err != nil {
			return fmt.Errorf("安装 Rook-Ceph CSI 失败: %w", err)
		}

	case "local-path-provisioner":
		// 安装 local-path-provisioner
		localPathCmd := `
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.31/deploy/local-path-storage.yaml
`
		_, err := m.RunSSHCommand(m.Config.Master.Name, localPathCmd)
		if err != nil {
			return fmt.Errorf("安装 local-path-provisioner 失败: %w", err)
		}

		// 将 local-path 设置为默认存储类
		defaultStorageClassCmd := `
kubectl patch storageclass local-path -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
`
		_, err = m.RunSSHCommand(m.Config.Master.Name, defaultStorageClassCmd)
		if err != nil {
			return fmt.Errorf("将 local-path 设置为默认存储类失败: %w", err)
		}

	case "none":
		return nil

	default:
		return fmt.Errorf("不支持的 CSI 类型: %s", m.CSIType)
	}

	return nil
}

// InstallLoadBalancer 安装 LoadBalancer (MetalLB)
func (m *Manager) InstallLoadBalancer() error {
	metallbCmd := `
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.9/config/manifests/metallb-native.yaml
`
	_, err := m.RunSSHCommand(m.Config.Master.Name, metallbCmd)
	if err != nil {
		return fmt.Errorf("安装 MetalLB 失败: %w", err)
	}

	// 等待MetalLB部署完成
	waitCmd := `
kubectl wait --namespace metallb-system --for=condition=ready pod --selector=app=metallb --timeout=90s
`
	_, err = m.RunSSHCommand(m.Config.Master.Name, waitCmd)
	if err != nil {
		return fmt.Errorf("等待 MetalLB 部署完成失败: %w", err)
	}

	// 获取节点IP地址范围
	ipRange, err := m.getMetalLBAddressRange()
	if err != nil {
		return fmt.Errorf("获取 MetalLB 地址范围失败: %w", err)
	}

	// 导入配置模板
	configYAML := strings.Replace(metallb.CONFIG_YAML, "# - 192.168.64.200 - 192.168.64.250", fmt.Sprintf("- %s", ipRange), 1)

	// 创建配置文件
	configFile := "/tmp/metallb-config.yaml"
	createConfigCmd := fmt.Sprintf("cat <<EOF | tee %s\n%sEOF", configFile, configYAML)
	_, err = m.RunSSHCommand(m.Config.Master.Name, createConfigCmd)
	if err != nil {
		return fmt.Errorf("创建 MetalLB 配置文件失败: %w", err)
	}

	// 应用配置
	applyConfigCmd := fmt.Sprintf("kubectl apply -f %s", configFile)
	_, err = m.RunSSHCommand(m.Config.Master.Name, applyConfigCmd)
	if err != nil {
		return fmt.Errorf("应用 MetalLB 配置失败: %w", err)
	}

	return nil
}

// getMetalLBAddressRange 获取适合MetalLB的IP地址范围
func (m *Manager) getMetalLBAddressRange() (string, error) {
	// 获取主节点IP地址
	ipCmd := "ip -4 addr show | grep inet | grep -v '127.0.0.1' | head -1 | awk '{print $2}' | cut -d/ -f1"
	output, err := m.RunSSHCommand(m.Config.Master.Name, ipCmd)
	if err != nil {
		return "", fmt.Errorf("获取节点IP地址失败: %w", err)
	}

	// 解析IP地址
	ip := strings.TrimSpace(output)
	ipParts := strings.Split(ip, ".")
	if len(ipParts) != 4 {
		return "", fmt.Errorf("IP地址格式错误: %s", ip)
	}

	// 使用相同子网的一段IP地址作为LoadBalancer IP池
	// 例如: 192.168.64.100 -> 192.168.64.200-192.168.64.250
	prefix := strings.Join(ipParts[:3], ".")
	startIP := 200
	endIP := 250

	return fmt.Sprintf("%s.%d - %s.%d", prefix, startIP, prefix, endIP), nil
}

// AddNode 添加新节点到集群
func (m *Manager) AddNode(role string, cpu int, memory int, disk int) error {
	// 确定节点名称
	nodeName := ""
	index := 0
	if role == cluster.RoleMaster {
		// 目前只支持单 master
		return fmt.Errorf("目前仅支持单 Master 节点")
	} else {
		for _, worker := range m.Cluster.Workers {
			suffix := strings.Split(strings.TrimPrefix(worker.Name, m.Config.Prefix()), "-")[1]
			suffixInt, err := strconv.Atoi(suffix)
			if err != nil {
				return fmt.Errorf("节点 %s 的索引转换失败: %w", worker.Name, err)
			}
			if suffixInt > index {
				index = suffixInt
			}
		}
	}

	nodeName = m.Config.GetWorkerVMName(index + 1)
	// 创建虚拟机
	cpuStr := strconv.Itoa(cpu)
	memoryStr := strconv.Itoa(memory) + "M"
	diskStr := strconv.Itoa(disk) + "G"
	fmt.Printf("创建节点 %s, cpu: %s, memory: %s, disk: %s\n", nodeName, cpuStr, memoryStr, diskStr)
	err := m.MultipassClient.CreateVM(nodeName, cpuStr, memoryStr, diskStr)
	if err != nil {
		return fmt.Errorf("创建节点 %s 失败: %w", nodeName, err)
	}

	// 更新节点信息并获取IP
	fmt.Printf("更新节点信息 %s\n", nodeName)
	err = m.AddNodeInfo(nodeName, cpu, memory, disk)
	if err != nil {
		return fmt.Errorf("更新节点信息失败: %w", err)
	}
	fmt.Printf("更新集群信息 %s\n", nodeName)
	err = cluster.SaveClusterInfomation(m.Cluster)
	if err != nil {
		return fmt.Errorf("更新集群信息失败: %w", err)
	}

	// 使用初始化器来设置环境
	fmt.Printf("初始化节点 %s\n", nodeName)
	initializer := environment.NewInitializerWithOptions(m, nodeName, m.InitOptions)
	if err := initializer.Initialize(); err != nil {
		return fmt.Errorf("初始化节点 %s 环境失败: %w", nodeName, err)
	}

	// 获取 join 命令
	fmt.Printf("获取 join 命令 %s\n", nodeName)
	joinCmd, err := m.joinCommand()
	if err != nil {
		return fmt.Errorf("获取 join 命令失败: %w", err)
	}
	fmt.Printf("加入集群 %s\n", nodeName)
	err = m.JoinWorkerNode(nodeName, joinCmd)
	if err != nil {
		return fmt.Errorf("节点 %s 加入集群失败: %w", nodeName, err)
	}
	return nil
}

// DeleteNode 从集群中删除节点
func (m *Manager) DeleteNode(nodeName string, force bool) error {
	// 检查节点是否存在
	nodeInfo := m.Cluster.GetNodeByName(nodeName)
	if nodeInfo == nil {
		return fmt.Errorf("节点 %s 不存在", nodeName)
	}

	if nodeInfo.Role == cluster.RoleMaster {
		return fmt.Errorf("不能删除 Master 节点，请先删除整个集群")
	}

	// 从 Kubernetes 中驱逐节点
	if !force {
		err := m.drainAndDeleteNode(nodeName)
		if err != nil {
			return err
		}
	}

	// 删除虚拟机
	fmt.Printf("正在删除节点 %s...\n", nodeName)
	err := m.MultipassClient.DeleteVM(nodeName)
	if err != nil {
		return fmt.Errorf("删除节点 %s 失败: %w", nodeName, err)
	}

	// 更新集群信息
	fmt.Printf("更新集群信息 %s\n", nodeName)
	m.Cluster.RemoveNode(nodeName)
	if err := cluster.SaveClusterInfomation(m.Cluster); err != nil {
		fmt.Printf("警告: 更新集群信息失败: %v\n", err)
	}

	fmt.Printf("节点 %s 已成功删除！\n", nodeName)
	return nil
}

func (m *Manager) drainAndDeleteNode(nodeName string) error {
	fmt.Printf("正在从 Kubernetes 集群中驱逐节点 %s...\n", nodeName)
	drainCmd := fmt.Sprintf(`kubectl drain %s --ignore-daemonsets --delete-emptydir-data --force`, nodeName)
	_, err := m.RunSSHCommand(m.Config.Master.Name, drainCmd)
	if err != nil {
		return fmt.Errorf("驱逐节点 %s 失败: %w", nodeName, err)
	}

	deleteNodeCmd := fmt.Sprintf(`kubectl delete node %s`, nodeName)
	_, err = m.RunSSHCommand(m.Config.Master.Name, deleteNodeCmd)
	if err != nil {
		return fmt.Errorf("从集群中删除节点 %s 失败: %w", nodeName, err)
	}
	return nil
}

// DeleteCluster 删除集群
func (m *Manager) DeleteCluster() error {
	fmt.Println("正在删除 Kubernetes 集群...")

	// 列出所有虚拟机
	prefix := m.Config.Prefix()
	vms, err := m.MultipassClient.ListVMs(prefix)
	if err != nil {
		return fmt.Errorf("列出虚拟机失败: %w", err)
	}

	// 找到集群相关的虚拟机并删除
	for _, vm := range vms {
		if strings.HasPrefix(vm, prefix) {
			fmt.Printf("删除节点: %s\n", vm)
			// 关闭SSH连接
			m.SSHManager.CloseClient(vm)

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

	// 删除集群信息文件
	homeDir, _ := os.UserHomeDir()
	clusterYaml := filepath.Join(homeDir, ".ohmykube", "cluster.yaml")
	if _, err := os.Stat(clusterYaml); err == nil {
		os.Remove(clusterYaml)
	}

	fmt.Println("集群删除完成！")
	return nil
}

// SetKubeadmConfigPath 设置自定义的kubeadm配置路径
func (m *Manager) SetKubeadmConfigPath(configPath string) {
	// 如果KubeadmConfig还未初始化，则不进行设置
	if m.KubeadmConfig == nil {
		fmt.Println("警告: KubeadmConfig尚未初始化，暂时无法设置自定义配置路径")
		return
	}
	m.KubeadmConfig.SetCustomConfig(configPath)
}
