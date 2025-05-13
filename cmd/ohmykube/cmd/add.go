package cmd

import (
	"github.com/spf13/cobra"
)

var (
	addNodeMemory int
	addNodeCPU    int
	addNodeDisk   int
	addNodeRole   string
	addNodeName   string
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "添加一个节点",
	Long:  `向已存在的 Kubernetes 集群中添加一个工作节点`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
	addCmd.Flags().IntVar(&addNodeMemory, "memory", 2048, "节点内存(MB)")
	addCmd.Flags().IntVar(&addNodeCPU, "cpu", 2, "节点CPU核心数")
	addCmd.Flags().IntVar(&addNodeDisk, "disk", 20, "节点磁盘空间(GB)")
	addCmd.Flags().StringVar(&addNodeRole, "role", "worker", "节点角色 (worker/master)")
}
