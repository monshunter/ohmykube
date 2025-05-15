package kubeadm

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigType 定义了配置的类型
type ConfigType string

const (
	// InitConfiguration 类型
	InitConfiguration ConfigType = "InitConfiguration"
	// ClusterConfiguration 类型
	ClusterConfiguration ConfigType = "ClusterConfiguration"
	// KubeletConfiguration 类型
	KubeletConfiguration ConfigType = "KubeletConfiguration"
	// KubeProxyConfiguration 类型
	KubeProxyConfiguration ConfigType = "KubeProxyConfiguration"
)

// YAMLDocument 表示一个YAML文档
type YAMLDocument map[string]any

// Config 管理kubeadm配置
type Config struct {
	// 各个配置部分
	InitConfig      YAMLDocument
	ClusterConfig   YAMLDocument
	KubeletConfig   YAMLDocument
	KubeProxyConfig YAMLDocument

	// 配置文件路径
	ConfigPath string
}

// NewConfig 创建一个新的配置实例，使用默认值
func NewConfig() *Config {
	return &Config{
		InitConfig:      loadDefaultInitConfig(),
		ClusterConfig:   loadDefaultClusterConfig(),
		KubeletConfig:   loadDefaultKubeletConfig(),
		KubeProxyConfig: loadDefaultKubeProxyConfig(),
	}
}

// LoadFromFile 从文件加载配置
func LoadFromFile(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	return LoadFromBytes(data)
}

// LoadFromBytes 从字节数据加载配置
func LoadFromBytes(data []byte) (*Config, error) {
	config := NewConfig()

	// 将数据分割为多个YAML文档
	docs, err := splitYAMLDocuments(data)
	if err != nil {
		return nil, fmt.Errorf("分割YAML文档失败: %w", err)
	}

	// 处理每个文档
	for _, doc := range docs {
		var document YAMLDocument
		if err := yaml.Unmarshal(doc, &document); err != nil {
			return nil, fmt.Errorf("解析YAML文档失败: %w", err)
		}

		kind, ok := document["kind"].(string)
		if !ok {
			continue
		}

		switch kind {
		case string(InitConfiguration):
			config.InitConfig = document
		case string(ClusterConfiguration):
			config.ClusterConfig = document
		case string(KubeletConfiguration):
			config.KubeletConfig = document
		case string(KubeProxyConfiguration):
			config.KubeProxyConfig = document
		}
	}

	return config, nil
}

// MergeWith 将另一个配置与当前配置合并
func (c *Config) MergeWith(other *Config) *Config {
	result := NewConfig()

	// 合并每个配置部分
	if other.InitConfig != nil {
		mergeMaps(result.InitConfig, other.InitConfig)
	} else {
		result.InitConfig = c.InitConfig
	}

	if other.ClusterConfig != nil {
		mergeMaps(result.ClusterConfig, other.ClusterConfig)
	} else {
		result.ClusterConfig = c.ClusterConfig
	}

	if other.KubeletConfig != nil {
		mergeMaps(result.KubeletConfig, other.KubeletConfig)
	} else {
		result.KubeletConfig = c.KubeletConfig
	}

	if other.KubeProxyConfig != nil {
		mergeMaps(result.KubeProxyConfig, other.KubeProxyConfig)
	} else {
		result.KubeProxyConfig = c.KubeProxyConfig
	}

	return result
}

// ToYAML 将配置转换为YAML字符串
func (c *Config) ToYAML() ([]byte, error) {
	var result []byte

	// 添加InitConfig
	if c.InitConfig != nil {
		data, err := yaml.Marshal(c.InitConfig)
		if err != nil {
			return nil, fmt.Errorf("序列化InitConfig失败: %w", err)
		}
		result = append(result, data...)
		result = append(result, []byte("---\n")...)
	}

	// 添加ClusterConfig
	if c.ClusterConfig != nil {
		data, err := yaml.Marshal(c.ClusterConfig)
		if err != nil {
			return nil, fmt.Errorf("序列化ClusterConfig失败: %w", err)
		}
		result = append(result, data...)
		result = append(result, []byte("---\n")...)
	}

	// 添加KubeletConfig
	if c.KubeletConfig != nil {
		data, err := yaml.Marshal(c.KubeletConfig)
		if err != nil {
			return nil, fmt.Errorf("序列化KubeletConfig失败: %w", err)
		}
		result = append(result, data...)
		result = append(result, []byte("---\n")...)
	}

	// 添加KubeProxyConfig
	if c.KubeProxyConfig != nil {
		data, err := yaml.Marshal(c.KubeProxyConfig)
		if err != nil {
			return nil, fmt.Errorf("序列化KubeProxyConfig失败: %w", err)
		}
		result = append(result, data...)
	}

	return result, nil
}

// SaveToFile 将配置保存到文件
func (c *Config) SaveToFile(filePath string) error {
	data, err := c.ToYAML()
	if err != nil {
		return fmt.Errorf("转换配置为YAML失败: %w", err)
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// GetConfig 获取指定类型的配置
func (c *Config) GetConfig(configType ConfigType) YAMLDocument {
	switch configType {
	case InitConfiguration:
		return c.InitConfig
	case ClusterConfiguration:
		return c.ClusterConfig
	case KubeletConfiguration:
		return c.KubeletConfig
	case KubeProxyConfiguration:
		return c.KubeProxyConfig
	default:
		return nil
	}
}

// SetConfig 设置指定类型的配置
func (c *Config) SetConfig(configType ConfigType, config YAMLDocument) {
	switch configType {
	case InitConfiguration:
		c.InitConfig = config
	case ClusterConfiguration:
		c.ClusterConfig = config
	case KubeletConfiguration:
		c.KubeletConfig = config
	case KubeProxyConfiguration:
		c.KubeProxyConfig = config
	}
}

// 辅助函数

// splitYAMLDocuments 将YAML数据分割为多个文档
func splitYAMLDocuments(data []byte) ([][]byte, error) {
	var docs [][]byte

	// 简单地按 "---" 分割
	chunks := [][]byte{}
	lines := [][]byte{}

	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.HasPrefix(bytes.TrimSpace(line), []byte("---")) {
			if len(lines) > 0 {
				chunks = append(chunks, bytes.Join(lines, []byte("\n")))
				lines = [][]byte{}
			}
		} else {
			lines = append(lines, line)
		}
	}

	if len(lines) > 0 {
		chunks = append(chunks, bytes.Join(lines, []byte("\n")))
	}

	for _, chunk := range chunks {
		if len(bytes.TrimSpace(chunk)) > 0 {
			docs = append(docs, chunk)
		}
	}

	return docs, nil
}

// mergeMaps 合并两个map
func mergeMaps(base, overlay YAMLDocument) {
	for k, v := range overlay {
		if baseVal, ok := base[k]; ok {
			// 如果两者都是map，递归合并
			if baseMap, ok := baseVal.(map[string]any); ok {
				if overlayMap, ok := v.(map[string]any); ok {
					mergeMaps(baseMap, overlayMap)
					continue
				}
			}
		}

		// 直接替换
		base[k] = v
	}
}
