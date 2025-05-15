package cmd

import (
	"fmt"
	"os"

	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/kubeconfig"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var downloadKubeconfigCmd = &cobra.Command{
	Use:   "download-kubeconfig",
	Short: "下载集群kubeconfig到本地",
	Long:  `将当前集群的kubeconfig文件下载到本地~/.kube目录，便于本地调试和管理集群`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 加载集群信息
		clusterInfo, err := cluster.LoadClusterInfo()
		if err != nil {
			return fmt.Errorf("加载集群信息失败: %w", err)
		}

		// 检查 master 节点信息
		if clusterInfo.Master.IP == "" {
			return fmt.Errorf("无法获取 master 节点 IP 地址")
		}

		// 读取SSH密钥
		sshKeyContent, err := os.ReadFile(sshKeyFile)
		if err != nil {
			return fmt.Errorf("读取SSH密钥失败: %w", err)
		}

		// 创建 SSH 客户端
		sshClient := ssh.NewClient(
			clusterInfo.Master.IP,
			clusterInfo.Master.SSHPort,
			clusterInfo.Master.SSHUser,
			password,
			string(sshKeyContent),
		)

		// 连接到 SSH 服务器
		if err := sshClient.Connect(); err != nil {
			return fmt.Errorf("连接到 master 节点失败: %w", err)
		}
		defer sshClient.Close()

		// 使用统一的kubeconfig下载功能
		kubeconfigPath, err := kubeconfig.DownloadToLocal(sshClient, clusterInfo.Name, "")
		if err != nil {
			return fmt.Errorf("下载 kubeconfig 失败: %w", err)
		}

		fmt.Printf("kubeconfig已下载到: %s\n", kubeconfigPath)
		fmt.Printf("可以使用以下命令访问集群:\n")
		fmt.Printf("export KUBECONFIG=%s\n", kubeconfigPath)
		fmt.Printf("kubectl get nodes\n")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(downloadKubeconfigCmd)
}
