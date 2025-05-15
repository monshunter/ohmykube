package cluster

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

	"github.com/monshunter/ohmykube/pkg/environment"
	"github.com/monshunter/ohmykube/pkg/kubeadm"
	"github.com/monshunter/ohmykube/pkg/kubeconfig"
	"github.com/monshunter/ohmykube/pkg/multipass"
	"github.com/monshunter/ohmykube/pkg/ssh"
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
		c.Workers[i].Name = c.GetWorkerVMName(i)
		c.Workers[i].ResourceConfig = workerResource
	}
	c.Master.Name = c.GetMasterVMName(0)
	c.Master.ResourceConfig = masterResource
	return c
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
	Config             *ClusterConfig
	MultipassClient    *multipass.Client
	KubeadmConfig      *kubeadm.KubeadmConfig
	SSHClients         map[string]*ssh.Client
	ClusterInfo        *ClusterInfo
	InitOptions        environment.InitOptions
	CNIType            string // 使用的CNI类型，默认为flannel
	CSIType            string // 使用的CSI类型，默认为local-path-provisioner
	DownloadKubeconfig bool   // 是否将kubeconfig下载到本地
}

// NewManager 创建新的集群管理器
func NewManager(config *ClusterConfig) (*Manager, error) {
	mpClient, err := multipass.NewClient(config.Image, config.Password, config.SSHConfig.GetSSHKey(), config.SSHConfig.GetSSHPubKey())
	if err != nil {
		return nil, fmt.Errorf("创建 Multipass 客户端失败: %w", err)
	}

	// 创建集群信息对象
	clusterInfo := &ClusterInfo{
		Name:       config.Name,
		K8sVersion: config.K8sVersion,
		Master: NodeInfo{
			Name:    config.Master.Name,
			Role:    RoleMaster,
			Status:  NodeStatusUnknown,
			CPU:     config.Master.CPU,
			Memory:  config.Master.Memory,
			Disk:    config.Master.Disk,
			SSHUser: "root",
			SSHPort: "22",
		},
		Workers: make([]NodeInfo, len(config.Workers)),
	}

	for i, worker := range config.Workers {
		clusterInfo.Workers[i] = NodeInfo{
			Name:    worker.Name,
			Role:    RoleWorker,
			Status:  NodeStatusUnknown,
			CPU:     worker.CPU,
			Memory:  worker.Memory,
			Disk:    worker.Disk,
			SSHUser: "root",
			SSHPort: "22",
		}
	}

	manager := &Manager{
		Config:             config,
		MultipassClient:    mpClient,
		SSHClients:         make(map[string]*ssh.Client),
		ClusterInfo:        clusterInfo,
		InitOptions:        environment.DefaultInitOptions(),
		CNIType:            "flannel",                // 默认使用flannel
		CSIType:            "local-path-provisioner", // 默认使用local-path-provisioner
		DownloadKubeconfig: true,                     // 默认下载kubeconfig
	}

	// 稍后在需要使用时创建SSH客户端和KubeadmConfig
	// KubeadmConfig将在初始化Master节点时设置

	return manager, nil
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

// UpdateClusterInfo 更新集群信息
func (m *Manager) UpdateClusterInfo() error {
	// 更新master节点信息
	masterIP, err := m.GetNodeIP(m.Config.Master.Name)
	if err != nil {
		return fmt.Errorf("获取Master节点IP失败: %w", err)
	}
	m.ClusterInfo.Master.IP = masterIP
	m.ClusterInfo.Master.Status = NodeStatusRunning
	m.ClusterInfo.Master.GenerateSSHCommand()

	// 等待Master节点SSH服务就绪
	if err := m.WaitForSSHReady(masterIP, m.ClusterInfo.Master.SSHPort, 10); err != nil {
		return fmt.Errorf("Master节点SSH服务未就绪: %w", err)
	}

	// 更新worker节点信息
	if len(m.Config.Workers) > 0 {
		for i := range m.Config.Workers {
			workerIP, err := m.GetNodeIP(m.Config.Workers[i].Name)
			if err != nil {
				return fmt.Errorf("获取Worker节点 %s IP失败: %w", m.Config.Workers[i].Name, err)
			}
			m.ClusterInfo.Workers[i].IP = workerIP
			m.ClusterInfo.Workers[i].Status = NodeStatusRunning
			m.ClusterInfo.Workers[i].GenerateSSHCommand()

			// 等待Worker节点SSH服务就绪
			if err := m.WaitForSSHReady(workerIP, m.ClusterInfo.Workers[i].SSHPort, 10); err != nil {
				return fmt.Errorf("worker节点 %s SSH服务未就绪: %w", m.Config.Workers[i].Name, err)
			}
		}
	}

	// 保存集群信息到本地文件
	return SaveClusterInfo(m.ClusterInfo)
}

// CreateSSHClient 为节点创建SSH客户端
func (m *Manager) CreateSSHClient(nodeName string) (*ssh.Client, error) {
	// 查找节点信息
	var nodeInfo *NodeInfo

	// 判断是否是master节点
	if nodeName == m.Config.Master.Name {
		nodeInfo = &m.ClusterInfo.Master
	} else {
		// 查找匹配的worker节点
		for i := range m.ClusterInfo.Workers {
			if m.Config.Workers[i].Name == nodeName {
				nodeInfo = &m.ClusterInfo.Workers[i]
				break
			}
		}
	}

	if nodeInfo == nil {
		return nil, fmt.Errorf("节点 %s 不存在", nodeName)
	}

	// 确保节点IP不为空
	if nodeInfo.IP == "" {
		return nil, fmt.Errorf("节点 %s 的IP地址不可用", nodeName)
	}

	// 创建SSH客户端
	client := ssh.NewClient(
		nodeInfo.IP,
		nodeInfo.SSHPort,
		nodeInfo.SSHUser,
		m.Config.Password,
		m.Config.SSHConfig.GetSSHKey(),
	)

	// 连接到SSH服务器，带重试机制
	maxRetries := 5
	retryDelay := 3 * time.Second
	var err error

	for i := 0; i < maxRetries; i++ {
		err = client.Connect()
		if err == nil {
			break
		}

		fmt.Printf("连接到节点 %s (%s) 失败，正在重试 (%d/%d)...\n", nodeName, nodeInfo.IP, i+1, maxRetries)
		time.Sleep(retryDelay)
	}

	if err != nil {
		return nil, fmt.Errorf("连接到节点 %s 失败，已重试 %d 次: %w", nodeName, maxRetries, err)
	}

	// 保存SSH客户端
	m.SSHClients[nodeName] = client
	return client, nil
}

// RunSSHCommand 通过SSH在节点上执行命令
func (m *Manager) RunSSHCommand(nodeName, command string) (string, error) {
	// 先检查是否已经有SSH客户端
	client, ok := m.SSHClients[nodeName]
	if !ok {
		// 如果没有，创建一个新的
		var err error
		client, err = m.CreateSSHClient(nodeName)
		if err != nil {
			return "", err
		}
	}

	// 执行命令
	return client.RunCommand(command)
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
	sshClient, ok := m.SSHClients[m.Config.Master.Name]
	if !ok {
		var err error
		sshClient, err = m.CreateSSHClient(m.Config.Master.Name)
		if err != nil {
			return "", fmt.Errorf("创建Master节点SSH客户端失败: %w", err)
		}
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

	// // 8. 安装 MetalLB
	// fmt.Println("安装 MetalLB LoadBalancer...")
	// err = m.InstallLoadBalancer()
	// if err != nil {
	// 	return fmt.Errorf("安装 LoadBalancer 失败: %w", err)
	// }

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
	if err := batchInitializer.InitializeWithConcurrencyLimit(2); err != nil {
		return fmt.Errorf("初始化环境失败: %w", err)
	}

	return nil
}

// InitializeMaster 初始化 master 节点
func (m *Manager) InitializeMaster() (string, error) {
	// 为master节点创建SSH客户端
	sshClient, err := m.CreateSSHClient(m.Config.Master.Name)
	if err != nil {
		return "", fmt.Errorf("为Master节点创建SSH客户端失败: %w", err)
	}

	// 创建KubeadmConfig
	m.KubeadmConfig = kubeadm.NewKubeadmConfig(sshClient, m.Config.K8sVersion, m.Config.Master.Name)

	// 使用新的配置系统生成配置文件并初始化Master
	err = m.KubeadmConfig.InitMaster()
	if err != nil {
		return "", fmt.Errorf("初始化 Master 节点失败: %w", err)
	}

	// 获取加入集群的命令
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
		// 直接使用RunSSHCommand而不是KubeadmConfig.JoinNode
		_, err := m.RunSSHCommand(m.Config.Workers[i].Name, joinCommand)
		if err != nil {
			return fmt.Errorf("将 Worker 节点 %s 加入集群失败: %w", m.Config.Workers[i].Name, err)
		}
	}
	return nil
}

// InstallCNI 安装 CNI
func (m *Manager) InstallCNI() error {
	// 确保master节点的SSH客户端已创建
	_, ok := m.SSHClients[m.Config.Master.Name]
	if !ok {
		_, err := m.CreateSSHClient(m.Config.Master.Name)
		if err != nil {
			return fmt.Errorf("为 Master 节点创建 SSH 客户端失败: %w", err)
		}
	}

	switch m.CNIType {
	case "cilium":
		// 安装 Cilium
		ciliumCmd := `
kubectl create -f https://raw.githubusercontent.com/cilium/cilium/v1.14/install/kubernetes/quick-install.yaml
`
		_, err := m.RunSSHCommand(m.Config.Master.Name, ciliumCmd)
		if err != nil {
			return fmt.Errorf("安装 Cilium CNI 失败: %w", err)
		}

	case "flannel":
		// 获取集群配置的podCIDR
		getPodCIDRCmd := `kubectl cluster-info dump | grep -m 1 cluster-cidr | grep -o '[0-9]\+\.[0-9]\+\.[0-9]\+\.[0-9]\+/[0-9]\+'`
		podCIDR := "10.244.0.0/16" // 默认值
		output, err := m.RunSSHCommand(m.Config.Master.Name, getPodCIDRCmd)
		if err != nil {
			fmt.Printf("警告: 无法获取集群podCIDR，将使用默认值 %s: %v\n", podCIDR, err)
		} else {
			if strings.TrimSpace(output) != "" {
				podCIDR = strings.TrimSpace(output)
				fmt.Printf("从集群获取到podCIDR: %s\n", podCIDR)
			}
		}

		// 检查Flannel helm仓库是否已添加
		checkRepoCmd := `helm repo list | grep -q "flannel" && echo "已添加" || echo "未添加"`
		output, err = m.RunSSHCommand(m.Config.Master.Name, checkRepoCmd)
		if err != nil {
			return fmt.Errorf("检查Flannel Helm仓库失败: %w", err)
		}

		// 如果仓库未添加，则添加
		if strings.TrimSpace(output) != "已添加" {
			fmt.Println("添加Flannel Helm仓库...")
			_, err = m.RunSSHCommand(m.Config.Master.Name, `
helm repo add flannel https://flannel-io.github.io/flannel/
helm repo update
`)
			if err != nil {
				return fmt.Errorf("添加Flannel Helm仓库失败: %w", err)
			}
		} else {
			fmt.Println("Flannel Helm仓库已存在，跳过添加")
		}

		// 检查kube-flannel命名空间是否已存在
		checkNsCmd := `kubectl get ns kube-flannel -o name 2>/dev/null || echo ""`
		output, err = m.RunSSHCommand(m.Config.Master.Name, checkNsCmd)
		if err != nil {
			return fmt.Errorf("检查kube-flannel命名空间失败: %w", err)
		}

		// 创建命名空间并设置标签（如果需要）
		if strings.TrimSpace(output) == "" {
			fmt.Println("创建kube-flannel命名空间...")
			_, err = m.RunSSHCommand(m.Config.Master.Name, `
kubectl create ns kube-flannel
kubectl label --overwrite ns kube-flannel pod-security.kubernetes.io/enforce=privileged
`)
			if err != nil {
				return fmt.Errorf("创建kube-flannel命名空间失败: %w", err)
			}
		} else {
			// 命名空间存在，只设置标签
			fmt.Println("kube-flannel命名空间已存在，只更新标签")
			_, err = m.RunSSHCommand(m.Config.Master.Name, `kubectl label --overwrite ns kube-flannel pod-security.kubernetes.io/enforce=privileged`)
			if err != nil {
				return fmt.Errorf("设置kube-flannel命名空间标签失败: %w", err)
			}
		}

		// 检查Flannel是否已安装
		checkFlannelCmd := `helm ls -n kube-flannel | grep -q "flannel" && echo "已安装" || echo "未安装"`
		output, err = m.RunSSHCommand(m.Config.Master.Name, checkFlannelCmd)
		if err != nil {
			return fmt.Errorf("检查Flannel安装状态失败: %w", err)
		}

		// 安装或升级Flannel
		if strings.TrimSpace(output) != "已安装" {
			fmt.Println("安装Flannel...")
			helmInstallCmd := fmt.Sprintf(`
helm install flannel --set podCidr="%s" --namespace kube-flannel flannel/flannel
`, podCIDR)
			_, err = m.RunSSHCommand(m.Config.Master.Name, helmInstallCmd)
			if err != nil {
				return fmt.Errorf("使用Helm安装Flannel失败: %w", err)
			}
		} else {
			fmt.Println("Flannel已安装，执行升级以更新配置...")
			helmUpgradeCmd := fmt.Sprintf(`
helm upgrade flannel --set podCidr="%s" --namespace kube-flannel flannel/flannel
`, podCIDR)
			_, err = m.RunSSHCommand(m.Config.Master.Name, helmUpgradeCmd)
			if err != nil {
				return fmt.Errorf("使用Helm升级Flannel失败: %w", err)
			}
		}

		// 等待Flannel就绪
		waitCmd := `
kubectl -n kube-flannel wait --for=condition=ready pod -l app=flannel --timeout=120s
`
		_, err = m.RunSSHCommand(m.Config.Master.Name, waitCmd)
		if err != nil {
			fmt.Printf("警告: 等待Flannel就绪超时，但将继续执行: %v\n", err)
		}

	default:
		return fmt.Errorf("不支持的 CNI 类型: %s", m.CNIType)
	}

	return nil
}

// InstallCSI 安装 CSI
func (m *Manager) InstallCSI() error {
	// 确保master节点的SSH客户端已创建
	_, ok := m.SSHClients[m.Config.Master.Name]
	if !ok {
		_, err := m.CreateSSHClient(m.Config.Master.Name)
		if err != nil {
			return fmt.Errorf("为 Master 节点创建 SSH 客户端失败: %w", err)
		}
	}

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
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.9/config/manifests/metallb-native.yaml
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

	return nil
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

	// 更新节点信息并获取IP
	err = m.UpdateClusterInfo()
	if err != nil {
		return fmt.Errorf("更新集群信息失败: %w", err)
	}

	// 使用初始化器来设置环境
	initializer := environment.NewInitializerWithOptions(m, nodeName, m.InitOptions)
	if err := initializer.Initialize(); err != nil {
		return fmt.Errorf("初始化节点 %s 环境失败: %w", nodeName, err)
	}

	// 获取 join 命令
	joinCmd := `sudo kubeadm token create --print-join-command`
	joinCommand, err := m.RunSSHCommand(m.Config.Master.Name, joinCmd)
	if err != nil {
		return fmt.Errorf("获取 join 命令失败: %w", err)
	}

	// 加入集群
	_, err = m.RunSSHCommand(nodeName, joinCommand)
	if err != nil {
		return fmt.Errorf("节点 %s 加入集群失败: %w", nodeName, err)
	}

	// 如果是 worker，添加存储标签
	if role == "worker" {
		labelCmd := fmt.Sprintf(`kubectl label node %s role=storage-node`, nodeName)
		_, err = m.RunSSHCommand(m.Config.Master.Name, labelCmd)
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
		drainCmd := fmt.Sprintf(`kubectl drain %s --ignore-daemonsets --delete-emptydir-data --force`, nodeName)
		_, err = m.RunSSHCommand(m.Config.Master.Name, drainCmd)
		if err != nil {
			return fmt.Errorf("驱逐节点 %s 失败: %w", nodeName, err)
		}

		deleteNodeCmd := fmt.Sprintf(`kubectl delete node %s`, nodeName)
		_, err = m.RunSSHCommand(m.Config.Master.Name, deleteNodeCmd)
		if err != nil {
			return fmt.Errorf("从集群中删除节点 %s 失败: %w", nodeName, err)
		}
	}

	// 删除虚拟机
	fmt.Printf("正在删除节点 %s...\n", nodeName)
	err = m.MultipassClient.DeleteVM(nodeName)
	if err != nil {
		return fmt.Errorf("删除节点 %s 失败: %w", nodeName, err)
	}

	// 关闭SSH连接
	if client, ok := m.SSHClients[nodeName]; ok {
		client.Close()
		delete(m.SSHClients, nodeName)
	}

	// 更新集群信息
	if err := m.UpdateClusterInfo(); err != nil {
		fmt.Printf("警告: 更新集群信息失败: %v\n", err)
	}

	fmt.Printf("节点 %s 已成功删除！\n", nodeName)
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
			// 关闭SSH连接
			if client, ok := m.SSHClients[vm]; ok {
				client.Close()
				delete(m.SSHClients, vm)
			}

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
