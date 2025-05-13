package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	harborMemory int
	harborCPU    int
	harborDisk   int
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "(可选) 创建一个本地harbor",
	Long:  `创建一个基于虚拟机的本地 Harbor 私有容器镜像仓库`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("开始创建本地 Harbor 实例...")
		fmt.Println("注意: 此功能尚未实现，将在未来版本中支持")
		// TODO: 实现 Harbor 创建逻辑
		// 1. 创建虚拟机
		// 2. 安装 Docker
		// 3. 安装 Harbor
		// 4. 配置证书和访问方式
		return nil
	},
}

func init() {
	registryCmd.Flags().IntVar(&harborMemory, "memory", 4096, "Harbor实例内存(MB)")
	registryCmd.Flags().IntVar(&harborCPU, "cpu", 2, "Harbor实例CPU核心数")
	registryCmd.Flags().IntVar(&harborDisk, "disk", 40, "Harbor实例磁盘空间(GB)")
}
