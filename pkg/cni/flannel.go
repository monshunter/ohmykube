package cni

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/clusterinfo"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// FlannelInstaller 负责安装 Flannel CNI
type FlannelInstaller struct {
	SSHClient  *ssh.Client
	MasterNode string
	PodCIDR    string
	Version    string
}

// NewFlannelInstaller 创建 Flannel 安装器
func NewFlannelInstaller(sshClient *ssh.Client, masterNode string) *FlannelInstaller {
	return &FlannelInstaller{
		SSHClient:  sshClient,
		MasterNode: masterNode,
		PodCIDR:    "10.244.0.0/16", // Flannel 默认 Pod CIDR
		Version:    "v0.26.7",       // Flannel 版本
	}
}

// Install 安装 Flannel CNI
func (f *FlannelInstaller) Install() error {
	// 确保 br_netfilter 模块已加载
	brNetfilterCmd := `
sudo modprobe br_netfilter
echo "br_netfilter" | sudo tee /etc/modules-load.d/br_netfilter.conf
`
	_, err := f.SSHClient.RunCommand(brNetfilterCmd)
	if err != nil {
		return fmt.Errorf("加载 br_netfilter 模块失败: %w", err)
	}

	// 使用ClusterInfo获取集群的Pod CIDR
	clusterInfo := clusterinfo.NewClusterInfo(f.SSHClient)
	podCIDR, err := clusterInfo.GetPodCIDR()
	if err == nil && podCIDR != "" {
		f.PodCIDR = podCIDR
		fmt.Printf("获取到集群Pod CIDR: %s\n", f.PodCIDR)
	}

	// 检查Flannel helm仓库是否已添加
	checkRepoCmd := `helm repo list | grep -q "flannel" && echo "已添加" || echo "未添加"`
	output, err := f.SSHClient.RunCommand(checkRepoCmd)
	if err != nil {
		return fmt.Errorf("检查Flannel Helm仓库失败: %w", err)
	}

	// 如果仓库未添加，则添加
	if strings.TrimSpace(output) != "已添加" {
		fmt.Println("添加Flannel Helm仓库...")
		_, err = f.SSHClient.RunCommand(`
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
	output, err = f.SSHClient.RunCommand(checkNsCmd)
	if err != nil {
		return fmt.Errorf("检查kube-flannel命名空间失败: %w", err)
	}

	// 创建命名空间并设置标签（如果需要）
	if strings.TrimSpace(output) == "" {
		fmt.Println("创建kube-flannel命名空间...")
		_, err = f.SSHClient.RunCommand(`
kubectl create ns kube-flannel
kubectl label --overwrite ns kube-flannel pod-security.kubernetes.io/enforce=privileged
`)
		if err != nil {
			return fmt.Errorf("创建kube-flannel命名空间失败: %w", err)
		}
	} else {
		// 命名空间存在，只设置标签
		fmt.Println("kube-flannel命名空间已存在，只更新标签")
		_, err = f.SSHClient.RunCommand(`kubectl label --overwrite ns kube-flannel pod-security.kubernetes.io/enforce=privileged`)
		if err != nil {
			return fmt.Errorf("设置kube-flannel命名空间标签失败: %w", err)
		}
	}

	// 检查Flannel是否已安装
	checkFlannelCmd := `helm ls -n kube-flannel | grep -q "flannel" && echo "已安装" || echo "未安装"`
	output, err = f.SSHClient.RunCommand(checkFlannelCmd)
	if err != nil {
		return fmt.Errorf("检查Flannel安装状态失败: %w", err)
	}

	// 安装或升级Flannel
	if strings.TrimSpace(output) != "已安装" {
		fmt.Println("安装Flannel...")
		helmInstallCmd := fmt.Sprintf(`
helm install flannel --set podCidr="%s" --namespace kube-flannel flannel/flannel
`, f.PodCIDR)
		_, err = f.SSHClient.RunCommand(helmInstallCmd)
		if err != nil {
			return fmt.Errorf("使用Helm安装Flannel失败: %w", err)
		}
	} else {
		fmt.Println("Flannel已安装，执行升级以更新配置...")
		helmUpgradeCmd := fmt.Sprintf(`
helm upgrade flannel --set podCidr="%s" --namespace kube-flannel flannel/flannel
`, f.PodCIDR)
		_, err = f.SSHClient.RunCommand(helmUpgradeCmd)
		if err != nil {
			return fmt.Errorf("使用Helm升级Flannel失败: %w", err)
		}
	}

	// 等待Flannel就绪
	waitCmd := `
kubectl -n kube-flannel wait --for=condition=ready pod -l app=flannel --timeout=120s
`
	_, err = f.SSHClient.RunCommand(waitCmd)
	if err != nil {
		fmt.Printf("警告: 等待Flannel就绪超时，但将继续执行: %v\n", err)
	}

	return nil
}

// SetPodCIDR 设置Pod CIDR
func (f *FlannelInstaller) SetPodCIDR(cidr string) {
	f.PodCIDR = cidr
}

// SetVersion 设置Flannel版本
func (f *FlannelInstaller) SetVersion(version string) {
	f.Version = version
}
