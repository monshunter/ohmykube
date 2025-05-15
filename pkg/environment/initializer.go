package environment

import (
	"fmt"
	"strings"
	"time"

	"github.com/monshunter/ohmykube/pkg/default/containerd"
	"github.com/monshunter/ohmykube/pkg/default/ipvs"
)

// Initializer 用于初始化单个Kubernetes节点环境
type Initializer struct {
	sshRunner SSHCommandRunner
	nodeName  string
	options   InitOptions
}

// NewInitializer 创建一个新的节点初始化器
func NewInitializer(sshRunner SSHCommandRunner, nodeName string) *Initializer {
	return &Initializer{
		sshRunner: sshRunner,
		nodeName:  nodeName,
		options:   DefaultInitOptions(),
	}
}

// NewInitializerWithOptions 创建一个新的节点初始化器并指定选项
func NewInitializerWithOptions(sshRunner SSHCommandRunner, nodeName string, options InitOptions) *Initializer {
	return &Initializer{
		sshRunner: sshRunner,
		nodeName:  nodeName,
		options:   options,
	}
}

// waitForAptLock 等待apt锁释放
func (i *Initializer) waitForAptLock() error {
	maxRetries := 30              // 最多等待30次
	retryDelay := 5 * time.Second // 每次等待5秒

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// 检查apt锁状态
		cmd := `if fuser /var/lib/apt/lists/lock /var/lib/dpkg/lock-frontend /var/lib/dpkg/lock 2>/dev/null; then
	echo "locked"
else
	echo "unlocked"
fi`

		output, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
		if err != nil {
			return fmt.Errorf("检查apt锁状态失败: %w", err)
		}

		// 如果不再锁定，则继续
		if attempt > 1 && strings.TrimSpace(output) == "unlocked" {
			fmt.Printf("在节点 %s 上apt锁已释放，继续安装\n", i.nodeName)
			return nil
		}

		// 如果仍然锁定，等待一段时间再试
		if attempt < maxRetries {
			fmt.Printf("在节点 %s 上apt仍然被锁定，等待释放 (尝试 %d/%d)...\n", i.nodeName, attempt, maxRetries)
			time.Sleep(retryDelay)
		} else {
			// 如果达到最大重试次数，尝试强制释放锁
			fmt.Printf("在节点 %s 上等待apt锁释放超时，尝试强制释放...\n", i.nodeName)

			killCmd := "sudo killall apt apt-get dpkg 2>/dev/null || true"
			_, err = i.sshRunner.RunSSHCommand(i.nodeName, killCmd)
			if err != nil {
				// 忽略killall可能的错误，因为进程可能不存在
			}

			unlockCmd := `sudo rm -f /var/lib/apt/lists/lock
						  sudo rm -f /var/cache/apt/archives/lock
						  sudo rm -f /var/lib/dpkg/lock
						  sudo rm -f /var/lib/dpkg/lock-frontend
						  sudo dpkg --configure -a`

			_, err = i.sshRunner.RunSSHCommand(i.nodeName, unlockCmd)
			if err != nil {
				return fmt.Errorf("强制释放apt锁失败: %w", err)
			}

			fmt.Printf("在节点 %s 上已强制释放apt锁\n", i.nodeName)

			// 修复：强制释放锁后额外等待一段时间确保锁真正释放
			extraWaitTime := 10 * time.Second
			fmt.Printf("在节点 %s 上等待额外的 %s 确保锁已完全释放...\n", i.nodeName, extraWaitTime)
			time.Sleep(extraWaitTime)

			// 再次检查锁状态
			output, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
			if err != nil {
				return fmt.Errorf("强制释放后检查apt锁状态失败: %w", err)
			}

			if strings.TrimSpace(output) != "unlocked" {
				return fmt.Errorf("在节点 %s 上强制释放apt锁后仍然被锁定", i.nodeName)
			}

			fmt.Printf("在节点 %s 上确认apt锁已完全释放\n", i.nodeName)
			return nil
		}
	}

	return fmt.Errorf("等待apt锁释放超时")
}

// DisableSwap 禁用swap
func (i *Initializer) DisableSwap() error {
	// 执行swapoff -a命令
	cmd := "sudo swapoff -a"
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上禁用swap失败: %w", i.nodeName, err)
	}

	// 修改/etc/fstab文件注释掉swap行
	cmd = "sudo sed -i '/swap/s/^/#/' /etc/fstab"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上修改/etc/fstab文件失败: %w", i.nodeName, err)
	}

	return nil
}

// EnableIPVS 启用IPVS模块
func (i *Initializer) EnableIPVS() error {
	// 创建/etc/modules-load.d/k8s.conf文件
	modulesFile := "/etc/modules-load.d/k8s.conf"
	cmd := fmt.Sprintf("cat <<EOF | sudo tee %s\n%sEOF", modulesFile, ipvs.K8S_MODULES_CONFIG)
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上创建%s文件失败: %w", i.nodeName, modulesFile, err)
	}

	// 加载内核模块
	modules := []string{
		"overlay",
		"br_netfilter",
		"ip_vs",
		"ip_vs_rr",
		"ip_vs_wrr",
		"ip_vs_sh",
		"nf_conntrack",
	}

	for _, module := range modules {
		cmd := fmt.Sprintf("sudo modprobe %s", module)
		_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
		if err != nil {
			return fmt.Errorf("在节点 %s 上加载%s模块失败: %w", i.nodeName, module, err)
		}
	}

	// 等待apt锁释放
	if err := i.waitForAptLock(); err != nil {
		return fmt.Errorf("在节点 %s 上安装IPVS工具失败: %w", i.nodeName, err)
	}

	// 安装IPVS工具
	cmd = "sudo apt-get install -y ipvsadm ipset"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上安装IPVS工具失败: %w", i.nodeName, err)
	}

	// 设置系统参数
	sysctlFile := "/etc/sysctl.d/k8s.conf"
	cmd = fmt.Sprintf("cat <<EOF | sudo tee %s\n%sEOF", sysctlFile, ipvs.K8S_SYSCTL_CONFIG)
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上创建%s文件失败: %w", i.nodeName, sysctlFile, err)
	}

	// 应用系统参数
	cmd = "sudo sysctl --system"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上应用系统参数失败: %w", i.nodeName, err)
	}

	return nil
}

// EnableNetworkBridge 启用网络桥接
func (i *Initializer) EnableNetworkBridge() error {
	// 创建/etc/modules-load.d/k8s.conf文件
	modulesFile := "/etc/modules-load.d/k8s.conf"
	modulesContent := "overlay\nbr_netfilter\n"
	cmd := fmt.Sprintf("cat <<EOF | sudo tee %s\n%sEOF", modulesFile, modulesContent)
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上创建%s文件失败: %w", i.nodeName, modulesFile, err)
	}

	// 加载基本模块
	modules := []string{
		"overlay",
		"br_netfilter",
	}

	for _, module := range modules {
		cmd := fmt.Sprintf("sudo modprobe %s", module)
		_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
		if err != nil {
			return fmt.Errorf("在节点 %s 上加载%s模块失败: %w", i.nodeName, module, err)
		}
	}

	// 设置基本系统参数
	sysctlFile := "/etc/sysctl.d/k8s.conf"
	sysctlContent := `net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
`
	cmd = fmt.Sprintf("cat <<EOF | sudo tee %s\n%sEOF", sysctlFile, sysctlContent)
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上创建%s文件失败: %w", i.nodeName, sysctlFile, err)
	}

	// 应用系统参数
	cmd = "sudo sysctl --system"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上应用系统参数失败: %w", i.nodeName, err)
	}

	return nil
}

// InstallContainerd 安装和配置containerd
func (i *Initializer) InstallContainerd() error {
	// 等待apt锁释放
	if err := i.waitForAptLock(); err != nil {
		return fmt.Errorf("在节点 %s 上安装containerd失败: %w", i.nodeName, err)
	}

	// 安装containerd
	cmd := "sudo apt-get install -y containerd"
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上安装containerd失败: %w", i.nodeName, err)
	}

	// 创建containerd配置目录
	cmd = "sudo mkdir -p /etc/containerd"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上创建containerd配置目录失败: %w", i.nodeName, err)
	}

	cmd = "sudo mkdir -p /etc/containerd/certs.d"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上创建containerd证书目录失败: %w", i.nodeName, err)
	}

	// 写入默认配置
	configFile := "/etc/containerd/config.toml"
	cmd = fmt.Sprintf("cat <<EOF | sudo tee %s\n%sEOF", configFile, containerd.CONTAINERD_CONFIG_1_7_24)
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上创建containerd配置文件失败: %w", i.nodeName, err)
	}

	// 重启containerd
	cmd = "sudo systemctl restart containerd"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上重启containerd失败: %w", i.nodeName, err)
	}

	// 启用containerd开机自启
	cmd = "sudo systemctl enable containerd"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上设置containerd开机自启失败: %w", i.nodeName, err)
	}

	return nil
}

// InstallK8sComponents 安装kubeadm、kubectl、kubelet
func (i *Initializer) InstallK8sComponents() error {
	// 等待apt锁释放
	if err := i.waitForAptLock(); err != nil {
		return fmt.Errorf("在节点 %s 上更新apt失败: %w", i.nodeName, err)
	}

	// 更新apt
	cmd := "sudo apt-get update"
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上更新apt失败: %w", i.nodeName, err)
	}

	// 等待apt锁释放
	if err := i.waitForAptLock(); err != nil {
		return fmt.Errorf("在节点 %s 上安装依赖失败: %w", i.nodeName, err)
	}

	// 安装依赖
	cmd = "sudo apt-get install -y apt-transport-https ca-certificates curl gpg"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上安装依赖失败: %w", i.nodeName, err)
	}

	// 创建证书目录
	cmd = "sudo mkdir -p -m 755 /etc/apt/keyrings"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上创建证书目录失败: %w", i.nodeName, err)
	}

	// 下载k8s公钥并导入
	cmd = "curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.33/deb/Release.key | sudo gpg --batch --yes --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上下载并导入K8s密钥失败: %w", i.nodeName, err)
	}

	// 添加k8s源
	cmd = "echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.33/deb/ /' | sudo tee /etc/apt/sources.list.d/kubernetes.list"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上添加Kubernetes源失败: %w", i.nodeName, err)
	}

	// 等待apt锁释放
	if err := i.waitForAptLock(); err != nil {
		return fmt.Errorf("在节点 %s 上更新apt失败: %w", i.nodeName, err)
	}

	// 再次更新apt
	cmd = "sudo apt-get update"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上更新apt失败: %w", i.nodeName, err)
	}

	// 等待apt锁释放
	if err := i.waitForAptLock(); err != nil {
		return fmt.Errorf("在节点 %s 上安装Kubernetes组件失败: %w", i.nodeName, err)
	}

	// 安装k8s组件
	cmd = "sudo apt-get install -y kubelet kubeadm kubectl"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上安装Kubernetes组件失败: %w", i.nodeName, err)
	}

	// 锁定版本
	cmd = "sudo apt-mark hold kubelet kubeadm kubectl"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上锁定Kubernetes组件版本失败: %w", i.nodeName, err)
	}

	// 启用kubelet
	cmd = "sudo systemctl enable --now kubelet"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上启用kubelet失败: %w", i.nodeName, err)
	}

	return nil
}

// InstallHelm 安装Helm
func (i *Initializer) InstallHelm() error {
	// 检查Helm是否已安装
	cmd := "command -v helm && echo 'Helm已安装' || echo 'Helm未安装'"
	output, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err == nil && output == "Helm已安装" {
		fmt.Printf("在节点 %s 上Helm已安装，跳过\n", i.nodeName)
		return nil
	}

	// 安装Helm
	cmd = `curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash`
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("在节点 %s 上安装Helm失败: %w", i.nodeName, err)
	}

	fmt.Printf("在节点 %s 上成功安装Helm\n", i.nodeName)
	return nil
}

// Initialize 执行所有初始化步骤
func (i *Initializer) Initialize() error {
	// 根据选项决定是否禁用swap
	if i.options.DisableSwap {
		if err := i.DisableSwap(); err != nil {
			return err
		}
	}

	// 根据选项决定是否启用IPVS
	if i.options.EnableIPVS {
		if err := i.EnableIPVS(); err != nil {
			return err
		}
	} else {
		// 如果不启用IPVS，仍然需要设置网络桥接
		if err := i.EnableNetworkBridge(); err != nil {
			return err
		}
	}

	// 安装容器运行时
	if i.options.ContainerRuntime == "containerd" {
		if err := i.InstallContainerd(); err != nil {
			return err
		}
	}

	if err := i.InstallK8sComponents(); err != nil {
		return err
	}

	// 安装Helm
	if err := i.InstallHelm(); err != nil {
		return err
	}

	return nil
}
