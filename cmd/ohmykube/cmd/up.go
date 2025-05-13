package cmd

import (
	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/spf13/cobra"
)

var (
	// 集群配置选项
	clusterName  string
	k8sVersion   string
	workersCount int
	masterMemory int
	masterCPU    int
	workerMemory int
	workerCPU    int
	masterDisk   int
	workerDisk   int
	vmImage      string
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "创建一个 k8s 集群 (包含Cilium, Rook, MetalLB)",
	Long: `创建一个基于虚拟机的 k8s 集群，包含以下组件：
- Cilium 作为 CNI
- Rook-Ceph 作为 CSI 
- MetalLB 作为 LoadBalancer 实现`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sshConfig, err := cluster.NewSSHConfig(password, sshKeyFile, sshPubKeyFile)
		if err != nil {
			return err
		}
		err = sshConfig.Init()
		if err != nil {
			return err
		}
		// 创建集群配置
		config := cluster.NewClusterConfig(clusterName, k8sVersion, workersCount, sshConfig,
			cluster.ResourceConfig{
				CPU:    masterCPU,
				Memory: masterMemory,
				Disk:   masterDisk,
			}, cluster.ResourceConfig{
				CPU:    workerCPU,
				Memory: workerMemory,
				Disk:   workerDisk,
			})
		config.K8sVersion = k8sVersion
		// 创建集群管理器
		manager, err := cluster.NewManager(config)
		if err != nil {
			return err
		}

		// 创建集群
		return manager.CreateCluster()
	},
}

func init() {
	// 添加命令行参数
	upCmd.Flags().StringVar(&clusterName, "name", "ohmykube", "集群名称")
	upCmd.Flags().IntVarP(&workersCount, "workers", "w", 0, "工作节点数量")
	upCmd.Flags().IntVar(&masterMemory, "master-memory", 4096, "Master节点内存(MB)")
	upCmd.Flags().IntVar(&masterCPU, "master-cpu", 2, "Master节点CPU核心数")
	upCmd.Flags().IntVar(&workerMemory, "worker-memory", 2048, "Worker节点内存(MB)")
	upCmd.Flags().IntVar(&workerCPU, "worker-cpu", 2, "Worker节点CPU核心数")
	upCmd.Flags().IntVar(&masterDisk, "master-disk", 20, "Master节点磁盘大小(GB)")
	upCmd.Flags().IntVar(&workerDisk, "worker-disk", 20, "Worker节点磁盘大小(GB)")
	upCmd.Flags().StringVar(&k8sVersion, "k8s-version", "1.33.0", "Kubernetes版本")
	upCmd.Flags().StringVar(&vmImage, "vm-image", "24.04", "虚拟机镜像")
}
