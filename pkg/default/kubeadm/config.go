package kubeadm

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigType defines the type of configuration
type ConfigType string

const (
	// InitConfiguration type
	InitConfiguration ConfigType = "InitConfiguration"
	// ClusterConfiguration type
	ClusterConfiguration ConfigType = "ClusterConfiguration"
	// KubeletConfiguration type
	KubeletConfiguration ConfigType = "KubeletConfiguration"
	// KubeProxyConfiguration type
	KubeProxyConfiguration ConfigType = "KubeProxyConfiguration"
)

// YAMLDocument represents a YAML document
type YAMLDocument map[string]any

// Config manages kubeadm configuration
type Config struct {
	// Configuration sections
	InitConfig      YAMLDocument
	ClusterConfig   YAMLDocument
	KubeletConfig   YAMLDocument
	KubeProxyConfig YAMLDocument

	// Configuration file path
	ConfigPath string
}

// NewConfig creates a new configuration instance with default values
func NewConfig(version string, proxyMode string, advertiseAddress string) *Config {
	return &Config{
		InitConfig:      loadInitConfig(advertiseAddress),
		ClusterConfig:   loadClusterConfig(version),
		KubeletConfig:   loadKubeletConfig(),
		KubeProxyConfig: loadKubeProxyConfig(proxyMode),
	}
}

// LoadFromFile loads configuration from a file
func LoadFromFile(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	return LoadFromBytes(data)
}

// LoadFromBytes loads configuration from byte data
func LoadFromBytes(data []byte) (*Config, error) {
	config := NewConfig("", "", "")

	// Split data into multiple YAML documents
	docs, err := splitYAMLDocuments(data)
	if err != nil {
		return nil, fmt.Errorf("failed to split YAML documents: %w", err)
	}

	// Process each document
	for _, doc := range docs {
		var document YAMLDocument
		if err := yaml.Unmarshal(doc, &document); err != nil {
			return nil, fmt.Errorf("failed to parse YAML document: %w", err)
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

// MergeWith merges another configuration with the current one
func (c *Config) MergeWith(other *Config) *Config {
	result := NewConfig("", "", "")

	// Merge each configuration section
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

// ToYAML converts the configuration to a YAML string
func (c *Config) ToYAML() ([]byte, error) {
	var result []byte

	// Add InitConfig
	if c.InitConfig != nil {
		data, err := yaml.Marshal(c.InitConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize InitConfig: %w", err)
		}
		result = append(result, data...)
		result = append(result, []byte("---\n")...)
	}

	// Add ClusterConfig
	if c.ClusterConfig != nil {
		data, err := yaml.Marshal(c.ClusterConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize ClusterConfig: %w", err)
		}
		result = append(result, data...)
		result = append(result, []byte("---\n")...)
	}

	// Add KubeletConfig
	if c.KubeletConfig != nil {
		data, err := yaml.Marshal(c.KubeletConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize KubeletConfig: %w", err)
		}
		result = append(result, data...)
		result = append(result, []byte("---\n")...)
	}

	// Add KubeProxyConfig
	if c.KubeProxyConfig != nil {
		data, err := yaml.Marshal(c.KubeProxyConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize KubeProxyConfig: %w", err)
		}
		result = append(result, data...)
	}

	return result, nil
}

// SaveToFile saves the configuration to a file
func (c *Config) SaveToFile(filePath string) error {
	data, err := c.ToYAML()
	if err != nil {
		return fmt.Errorf("failed to convert configuration to YAML: %w", err)
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}

	return nil
}

// GetConfig gets the configuration of the specified type
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

// SetConfig sets the configuration of the specified type
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

// splitYAMLDocuments splits YAML data into multiple documents
func splitYAMLDocuments(data []byte) ([][]byte, error) {
	sep := []byte("---\n")
	docs := bytes.Split(data, sep)

	// Filter out empty documents
	var result [][]byte
	for _, doc := range docs {
		if len(bytes.TrimSpace(doc)) > 0 {
			result = append(result, doc)
		}
	}

	return result, nil
}

// mergeMaps merges two maps, with overlay values taking precedence
func mergeMaps(base, overlay YAMLDocument) {
	for k, v := range overlay {
		if k == "kind" || k == "apiVersion" {
			continue
		}

		// If overlay value is a map, merge it recursively
		if overlayMap, ok := v.(map[string]any); ok {
			if baseMap, ok := base[k].(map[string]any); ok {
				// Create new maps for conversion
				baseYAML := make(YAMLDocument)
				overlayYAML := make(YAMLDocument)

				for bk, bv := range baseMap {
					baseYAML[bk] = bv
				}

				for ok, ov := range overlayMap {
					overlayYAML[ok] = ov
				}

				mergeMaps(baseYAML, overlayYAML)

				// Convert back to map[string]any
				newBaseMap := make(map[string]any)
				for bk, bv := range baseYAML {
					newBaseMap[bk] = bv
				}

				base[k] = newBaseMap
				continue
			}
		}

		// Otherwise, just set the value
		base[k] = v
	}
}
