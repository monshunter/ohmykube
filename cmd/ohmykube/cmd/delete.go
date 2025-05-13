package cmd

import (
	"github.com/spf13/cobra"
)

var (
	deleteNodeName string
	deleteForce    bool
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除一个节点",
	Long:  `从现有 Kubernetes 集群中删除一个节点`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
	deleteCmd.Flags().StringVar(&deleteNodeName, "name", "", "要删除的节点名称")
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "强制删除节点，即使它不能正常从集群中移除")
}
