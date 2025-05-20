package kubeadm

import (
	"fmt"
	"os"
)

// GenerateKubeadmConfig generates kubeadm config and saves it to a temporary file
// Parameters:
// - k8sVersion: Kubernetes version
// - podCIDR: Pod CIDR network
// - serviceCIDR: Service CIDR network
// - customConfigPath: Optional, custom config path (if provided, will be merged with default config)
// Returns:
// - generated config file path
// - error message
func GenerateKubeadmConfig(k8sVersion string, customConfigPath string, proxyMode string) (string, error) {
	// Create base config
	config := NewConfig(k8sVersion, proxyMode)

	// If custom config exists, load and merge it
	if customConfigPath != "" {
		customConfig, err := LoadFromFile(customConfigPath)
		if err != nil {
			return "", fmt.Errorf("failed to load custom config: %w", err)
		}

		config = config.MergeWith(customConfig)
	}

	// Update Kubernetes version and network settings
	if config.ClusterConfig != nil {
		if k8sVersion != "" {
			config.ClusterConfig["kubernetesVersion"] = k8sVersion
		}

		if networking, ok := config.ClusterConfig["networking"].(map[string]any); ok {
			networking["podSubnet"] = "10.244.0.0/16"
			networking["serviceSubnet"] = "10.96.0.0/12"
		}
	}

	// Create temporary file
	tmpfile, err := os.CreateTemp("", "kubeadm-config-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary config file: %w", err)
	}

	// Convert config to YAML and write to temporary file
	yamlData, err := config.ToYAML()
	if err != nil {
		os.Remove(tmpfile.Name())
		return "", fmt.Errorf("failed to generate YAML config: %w", err)
	}

	if _, err := tmpfile.Write(yamlData); err != nil {
		os.Remove(tmpfile.Name())
		return "", fmt.Errorf("failed to write config file: %w", err)
	}

	if err := tmpfile.Close(); err != nil {
		os.Remove(tmpfile.Name())
		return "", fmt.Errorf("failed to close config file: %w", err)
	}

	return tmpfile.Name(), nil
}

// LoadAndMergeConfigs loads and merges multiple config files
// Parameters:
// - configPaths: config file paths
// Returns:
// - merged config
// - error message
func LoadAndMergeConfigs(configPaths []string) (*Config, error) {
	if len(configPaths) == 0 {
		return NewConfig("", ""), nil
	}

	// Load first config as base
	baseConfig, err := LoadFromFile(configPaths[0])
	if err != nil {
		return nil, fmt.Errorf("failed to load base config file %s: %w", configPaths[0], err)
	}

	// Merge remaining configs
	for i := 1; i < len(configPaths); i++ {
		overlayConfig, err := LoadFromFile(configPaths[i])
		if err != nil {
			return nil, fmt.Errorf("failed to load config file %s: %w", configPaths[i], err)
		}

		baseConfig = baseConfig.MergeWith(overlayConfig)
	}

	return baseConfig, nil
}

// GetDefaultConfig gets the default kubeadm config
func GetDefaultConfig() *Config {
	return NewConfig("", "")
}
