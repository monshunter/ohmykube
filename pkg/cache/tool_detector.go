package cache

import (
	"context"
	"os/exec"
	"strings"

	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
)

// ToolDetector provides methods to detect available tools
type ToolDetector struct{}

// NewToolDetector creates a new tool detector
func NewToolDetector() *ToolDetector {
	return &ToolDetector{}
}

// DetectToolAvailability detects which tools are available locally and on controller node
func (td *ToolDetector) DetectToolAvailability(ctx context.Context, sshRunner interfaces.SSHRunner, controllerNode string) (*ToolAvailability, error) {
	availability := &ToolAvailability{}

	// Detect controller tools (only nerdctl for container operations)
	var err error
	availability.Nerdctl, err = td.isRemoteToolAvailable(sshRunner, controllerNode, "nerdctl")
	if err != nil {
		log.Warningf("Failed to check nerdctl on controller: %v", err)
	}

	availability.Helm, err = td.isRemoteToolAvailable(sshRunner, controllerNode, "helm")
	if err != nil {
		log.Warningf("Failed to check helm on controller: %v", err)
	}

	availability.Kubeadm, err = td.isRemoteToolAvailable(sshRunner, controllerNode, "kubeadm")
	if err != nil {
		log.Warningf("Failed to check kubeadm on controller: %v", err)
	}

	availability.Curl, err = td.isRemoteToolAvailable(sshRunner, controllerNode, "curl")
	if err != nil {
		log.Warningf("Failed to check curl on controller: %v", err)
	}

	return availability, nil
}

// IsLocalToolAvailable checks if a tool is available locally (internal use)
func (td *ToolDetector) IsLocalToolAvailable(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

// isRemoteToolAvailable checks if a tool is available on a remote node
func (td *ToolDetector) isRemoteToolAvailable(sshRunner interfaces.SSHRunner, nodeName, tool string) (bool, error) {
	cmd := "command -v " + tool + " >/dev/null 2>&1 && echo 'available' || echo 'not_available'"
	output, err := sshRunner.RunCommand(nodeName, cmd)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) == "available", nil
}

// DetermineOptimalStrategy determines the optimal image management strategy based on tool availability
func (td *ToolDetector) DetermineOptimalStrategy(availability *ToolAvailability, config ImageManagementConfig) ImageManagementStrategy {
	if config.ForceStrategy {
		return config.Strategy
	}

	// If strategy is auto, determine the best one using consistent tool preference
	if config.Strategy == ImageManagementAuto {
		return td.selectBestStrategyConsistent(availability)
	}

	// Check if the preferred strategy is viable
	if td.isStrategyViable(config.Strategy, availability) {
		return config.Strategy
	}

	// Try fallback strategies
	for _, fallback := range config.FallbackOrder {
		if td.isStrategyViable(fallback, availability) {
			log.Warningf("Falling back to %s strategy", fallback.String())
			return fallback
		}
	}

	// Default to controller strategy as last resort
	log.Warningf("No viable strategy found, defaulting to controller strategy")
	return ImageManagementController
}

// selectBestStrategyConsistent selects the best strategy using consistent tool preference: controller → target
func (td *ToolDetector) selectBestStrategyConsistent(availability *ToolAvailability) ImageManagementStrategy {
	// Always prefer controller → target order for automatic policy
	preferredStrategies := []ImageManagementStrategy{ImageManagementController, ImageManagementTarget}

	for _, strategy := range preferredStrategies {
		if td.isStrategyViable(strategy, availability) {
			log.Debugf("Selected %s strategy (consistent automatic policy)", strategy.String())
			return strategy
		}
	}

	// This should never happen since target strategy is always viable
	log.Warningf("No viable strategy found, defaulting to target strategy")
	return ImageManagementTarget
}

// isStrategyViable checks if a strategy is viable given tool availability
func (td *ToolDetector) isStrategyViable(strategy ImageManagementStrategy, availability *ToolAvailability) bool {
	switch strategy {
	case ImageManagementController:
		// Need nerdctl on controller for reliable container operations
		return availability.Nerdctl
	case ImageManagementTarget:
		// Always viable as we can install tools on target if needed
		return true
	default:
		return false
	}
}

// LogToolAvailability logs the detected tool availability for debugging
func (td *ToolDetector) LogToolAvailability(availability *ToolAvailability) {
	log.Debugf("Tool availability detected:")
	log.Debugf("  Controller tools:")
	log.Debugf("    Nerdctl: %t", availability.Nerdctl)
	log.Debugf("    Helm: %t, Kubeadm: %t, Curl: %t", availability.Helm, availability.Kubeadm, availability.Curl)
}

// DetermineOptimalStrategyForSource determines the optimal strategy based on specific image source requirements
func (td *ToolDetector) DetermineOptimalStrategyForSource(source ImageSource, availability *ToolAvailability, config ImageManagementConfig) ImageManagementStrategy {
	if config.ForceStrategy {
		return config.Strategy
	}

	// For automatic policy, use the same strategy selection as the general case
	// but validate that the selected strategy can handle this specific source
	if config.Strategy == ImageManagementAuto {
		return td.selectBestStrategyForSource(source, availability)
	}

	// Check if the preferred strategy can handle this source
	if td.CanStrategyHandleSource(config.Strategy, source, availability) {
		return config.Strategy
	}

	// Try fallback strategies
	for _, fallback := range config.FallbackOrder {
		if td.CanStrategyHandleSource(fallback, source, availability) {
			log.Warningf("Falling back to %s strategy for %s source", fallback.String(), source.Type)
			return fallback
		}
	}

	// Default to target strategy as last resort
	log.Warningf("No viable strategy found for %s source, defaulting to target strategy", source.Type)
	return ImageManagementTarget
}

// selectBestStrategyForSource selects the best strategy that can handle the specific source
func (td *ToolDetector) selectBestStrategyForSource(source ImageSource, availability *ToolAvailability) ImageManagementStrategy {
	// Always prefer controller → target order for automatic policy
	preferredStrategies := []ImageManagementStrategy{ImageManagementController, ImageManagementTarget}

	for _, strategy := range preferredStrategies {
		if td.CanStrategyHandleSource(strategy, source, availability) {
			log.Debugf("Selected %s strategy for %s source (automatic policy)", strategy.String(), source.Type)
			return strategy
		}
	}

	// This should never happen since target strategy is always viable
	log.Warningf("No strategy found for %s source, defaulting to target strategy", source.Type)
	return ImageManagementTarget
}

// CanStrategyHandleSource checks if a strategy can handle a specific image source
func (td *ToolDetector) CanStrategyHandleSource(strategy ImageManagementStrategy, source ImageSource, availability *ToolAvailability) bool {
	// Check what tools are required for this specific source type
	requiredTools := td.getRequiredToolsForSource(source)
	return td.hasRequiredToolsForStrategy(strategy, requiredTools, availability)
}

// getRequiredToolsForSource returns the tools required for a specific image source type
func (td *ToolDetector) getRequiredToolsForSource(source ImageSource) []string {
	switch source.Type {
	case "helm":
		return []string{"helm"} // Only need helm for templating and image discovery
	case "kubeadm":
		return []string{"kubeadm"} // Only need kubeadm for image discovery
	case "manifest":
		return []string{"curl"} // Only need curl to download manifests
	default:
		return []string{"container"} // Default: need container runtime
	}
}

// hasRequiredToolsForStrategy checks if a strategy has all required tools
func (td *ToolDetector) hasRequiredToolsForStrategy(strategy ImageManagementStrategy, requiredTools []string, availability *ToolAvailability) bool {
	for _, tool := range requiredTools {
		if !td.hasToolForStrategy(strategy, tool, availability) {
			return false
		}
	}
	return true
}

// hasToolForStrategy checks if a specific tool is available for a strategy
func (td *ToolDetector) hasToolForStrategy(strategy ImageManagementStrategy, tool string, availability *ToolAvailability) bool {
	switch strategy {
	case ImageManagementController:
		switch tool {
		case "container":
			return availability.Nerdctl
		case "helm":
			return availability.Helm
		case "kubeadm":
			return availability.Kubeadm
		case "curl":
			return availability.Curl
		}
	case ImageManagementTarget:
		// Target strategy is always viable as we can install tools if needed
		return true
	}
	return false
}

// GetPreferredContainerTool returns the preferred container tool for a given strategy
func (td *ToolDetector) GetPreferredContainerTool(strategy ImageManagementStrategy, availability *ToolAvailability) string {
	switch strategy {
	case ImageManagementController:
		if availability.Nerdctl {
			return "nerdctl"
		}
	}
	return "nerdctl" // Default fallback - nerdctl is required
}
