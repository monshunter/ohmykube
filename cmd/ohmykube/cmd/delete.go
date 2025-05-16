package cmd

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/manager"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var (
	deleteForce bool
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除一个或者多个节点",
	Long:  `从现有 Kubernetes 集群中删除一个或者多个节点`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// args 中获取节点名称
		deleteNodeNames := args
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
		// 删除节点
		for _, nodeName := range deleteNodeNames {
			nodeName = strings.TrimSpace(nodeName)
			if err := manager.DeleteNode(nodeName, deleteForce); err != nil {
				return fmt.Errorf("删除节点 %s 失败: %w", nodeName, err)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "强制删除节点，即使它不能正常从集群中移除")
}
