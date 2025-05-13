package cmd

import (
	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "删除一个 k8s 集群",
	Long:  `删除已创建的 Kubernetes 集群和相关的所有虚拟机资源`,
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

		// 删除集群
		return manager.DeleteCluster()
	},
}

func init() {
	// 暂时没有特定标志
}
