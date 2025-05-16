package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	password      string
	sshKeyFile    string
	sshPubKeyFile string
)

var rootCmd = &cobra.Command{
	Use:   "ohmykube",
	Short: "OhMyKube - 在本地快速搭建真实 Kubernetes 集群的工具",
	Long: `OhMyKube 是一个基于 Multipass 和 kubeadm 的工具，用于在开发者电脑上
快速搭建基于虚拟机实现的真实 Kubernetes 集群，包含 Cilium(CNI)、Rook(CSI) 和 MetalLB(LB)。`,
	// 如果需要在执行前做一些准备工作，可以添加 PersistentPreRun
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// 初始化工作
	},
}

// Execute 添加所有子命令到根命令并设置标志，这是 main.go 调用的入口
func Execute() error {
	return rootCmd.Execute()
}

const (
	defaultPassword = "ohmykube123"
)

var (
	defaultSSHKeyFile    string
	defaultSSHPubKeyFile string
	clusterName          string
)

func init() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("获取用户主目录失败: %v", err)
		os.Exit(1)
	}
	defaultSSHKeyFile = homeDir + "/.ssh/id_rsa"
	defaultSSHPubKeyFile = homeDir + "/.ssh/id_rsa.pub"
	// 这里可以添加全局标志
	rootCmd.PersistentFlags().StringVar(&password, "password", defaultPassword, "密码")
	rootCmd.PersistentFlags().StringVar(&sshKeyFile, "ssh-key", defaultSSHKeyFile, "SSH私钥文件")
	rootCmd.PersistentFlags().StringVar(&sshPubKeyFile, "ssh-pub-key", defaultSSHPubKeyFile, "SSH公钥文件")
	rootCmd.PersistentFlags().StringVar(&clusterName, "name", "ohmykube", "集群名称")
	// 添加子命令
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(registryCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(deleteCmd)
}
