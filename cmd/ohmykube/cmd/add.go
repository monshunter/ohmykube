package cmd

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/manager"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	addNodeMemory int
	addNodeCPU    int
	addNodeDisk   int
	addNodeRole   string
	count         int
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "添加一个或者多个节点",
	Long:  `向已存在的 Kubernetes 集群中添加一个或者多个工作节点`,
	Args:  cobra.MinimumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		// 加载集群信息
		clusterInfo, err := cluster.LoadClusterInfomation(clusterName)
		if err != nil {
			return fmt.Errorf("加载集群信息失败: %w", err)
		}

		// 读取SSH配置
		sshConfig, err := ssh.NewSSHConfig(password, sshKeyFile, sshPubKeyFile)
		if err != nil {
			return fmt.Errorf("创建SSH配置失败: %w", err)
		}
		// 创建集群配置
		config := &cluster.Config{
			Name:       clusterInfo.Name,
			K8sVersion: clusterInfo.K8sVersion,
			Master:     cluster.Node{Name: clusterInfo.Master.Name},
		}

		// 创建集群管理器
		manager, err := manager.NewManager(config, sshConfig, clusterInfo)
		if err != nil {
			return fmt.Errorf("创建集群管理器失败: %w", err)
		}
		defer manager.CloseSSHClient()
		for range count {
			// 添加节点（注意这里不需要显式设置InitOptions，因为NewManager中已经使用DefaultInitOptions初始化了）
			if err := manager.AddNode(addNodeRole, addNodeCPU, addNodeMemory, addNodeDisk); err != nil {
				return fmt.Errorf("添加节点失败: %w", err)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().IntVar(&addNodeMemory, "memory", 2048, "节点内存(MB)")
	addCmd.Flags().IntVar(&addNodeCPU, "cpu", 2, "节点CPU核心数")
	addCmd.Flags().IntVar(&addNodeDisk, "disk", 20, "节点磁盘空间(GB)")
	addCmd.Flags().StringVar(&addNodeRole, "role", "worker", "节点角色 (worker/master)")
	addCmd.Flags().IntVar(&count, "count", 1, "添加的节点数量")
}
