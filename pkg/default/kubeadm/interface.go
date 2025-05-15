package kubeadm

import (
	"fmt"
	"os"
)

// GenerateKubeadmConfig 生成kubeadm配置并保存到临时文件
// 参数:
// - k8sVersion: Kubernetes版本
// - podCIDR: Pod CIDR网络
// - serviceCIDR: Service CIDR网络
// - customConfigPath: 可选，自定义配置路径(如果提供，将与默认配置合并)
// 返回:
// - 生成的配置文件路径
// - 错误信息
func GenerateKubeadmConfig(k8sVersion, podCIDR, serviceCIDR string, customConfigPath string) (string, error) {
	// 创建基础配置
	config := NewConfig()

	// 如果自定义配置存在，则加载并合并
	if customConfigPath != "" {
		customConfig, err := LoadFromFile(customConfigPath)
		if err != nil {
			return "", fmt.Errorf("加载自定义配置失败: %w", err)
		}

		config = config.MergeWith(customConfig)
	}

	// 更新Kubernetes版本和网络设置
	if config.ClusterConfig != nil {
		if k8sVersion != "" {
			config.ClusterConfig["kubernetesVersion"] = k8sVersion
		}

		if networking, ok := config.ClusterConfig["networking"].(map[string]any); ok {
			if podCIDR != "" {
				networking["podSubnet"] = podCIDR
			}
			if serviceCIDR != "" {
				networking["serviceSubnet"] = serviceCIDR
			}
		}
	}

	// 创建临时文件
	tmpfile, err := os.CreateTemp("", "kubeadm-config-*.yaml")
	if err != nil {
		return "", fmt.Errorf("创建临时配置文件失败: %w", err)
	}

	// 将配置转换为YAML并写入临时文件
	yamlData, err := config.ToYAML()
	if err != nil {
		os.Remove(tmpfile.Name())
		return "", fmt.Errorf("生成YAML配置失败: %w", err)
	}

	if _, err := tmpfile.Write(yamlData); err != nil {
		os.Remove(tmpfile.Name())
		return "", fmt.Errorf("写入配置文件失败: %w", err)
	}

	if err := tmpfile.Close(); err != nil {
		os.Remove(tmpfile.Name())
		return "", fmt.Errorf("关闭配置文件失败: %w", err)
	}

	return tmpfile.Name(), nil
}

// LoadAndMergeConfigs 加载并合并多个配置文件
// 参数:
// - configPaths: 配置文件路径列表
// 返回:
// - 合并后的配置
// - 错误信息
func LoadAndMergeConfigs(configPaths []string) (*Config, error) {
	if len(configPaths) == 0 {
		return NewConfig(), nil
	}

	// 加载第一个配置作为基础
	baseConfig, err := LoadFromFile(configPaths[0])
	if err != nil {
		return nil, fmt.Errorf("加载基础配置文件 %s 失败: %w", configPaths[0], err)
	}

	// 合并其余配置
	for i := 1; i < len(configPaths); i++ {
		overlayConfig, err := LoadFromFile(configPaths[i])
		if err != nil {
			return nil, fmt.Errorf("加载配置文件 %s 失败: %w", configPaths[i], err)
		}

		baseConfig = baseConfig.MergeWith(overlayConfig)
	}

	return baseConfig, nil
}

// GetDefaultConfig 获取默认的kubeadm配置
func GetDefaultConfig() *Config {
	return NewConfig()
}
