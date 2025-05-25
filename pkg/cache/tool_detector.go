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

	// Detect local tools
	availability.LocalDocker = td.IsLocalToolAvailable("docker")
	availability.LocalPodman = td.IsLocalToolAvailable("podman")
	availability.LocalNerdctl = td.IsLocalToolAvailable("nerdctl")
	availability.LocalHelm = td.IsLocalToolAvailable("helm")
	availability.LocalKubeadm = td.IsLocalToolAvailable("kubeadm")
	availability.LocalCurl = td.IsLocalToolAvailable("curl")

	// Detect controller tools
	var err error
	availability.ControllerDocker, err = td.isRemoteToolAvailable(sshRunner, controllerNode, "docker")
	if err != nil {
		log.Infof("Warning: Failed to check docker on controller: %v", err)
	}

	availability.ControllerPodman, err = td.isRemoteToolAvailable(sshRunner, controllerNode, "podman")
	if err != nil {
		log.Infof("Warning: Failed to check podman on controller: %v", err)
	}

	availability.ControllerNerdctl, err = td.isRemoteToolAvailable(sshRunner, controllerNode, "nerdctl")
	if err != nil {
		log.Infof("Warning: Failed to check nerdctl on controller: %v", err)
	}

	availability.ControllerHelm, err = td.isRemoteToolAvailable(sshRunner, controllerNode, "helm")
	if err != nil {
		log.Infof("Warning: Failed to check helm on controller: %v", err)
	}

	availability.ControllerKubeadm, err = td.isRemoteToolAvailable(sshRunner, controllerNode, "kubeadm")
	if err != nil {
		log.Infof("Warning: Failed to check kubeadm on controller: %v", err)
	}

	availability.ControllerCurl, err = td.isRemoteToolAvailable(sshRunner, controllerNode, "curl")
	if err != nil {
		log.Infof("Warning: Failed to check curl on controller: %v", err)
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

	// If strategy is auto, determine the best one
	if config.Strategy == ImageManagementAuto {
		return td.selectBestStrategy(availability, config.FallbackOrder)
	}

	// Check if the preferred strategy is viable
	if td.isStrategyViable(config.Strategy, availability) {
		return config.Strategy
	}

	// Try fallback strategies
	for _, fallback := range config.FallbackOrder {
		if td.isStrategyViable(fallback, availability) {
			log.Infof("Falling back to %s strategy", fallback.String())
			return fallback
		}
	}

	// Default to controller strategy as last resort
	log.Infof("Warning: No viable strategy found, defaulting to controller strategy")
	return ImageManagementController
}

// selectBestStrategy selects the best strategy based on tool availability
func (td *ToolDetector) selectBestStrategy(availability *ToolAvailability, preferenceOrder []ImageManagementStrategy) ImageManagementStrategy {
	// Score each strategy based on tool availability
	scores := map[ImageManagementStrategy]int{
		ImageManagementLocal:      td.scoreLocalStrategy(availability),
		ImageManagementController: td.scoreControllerStrategy(availability),
		ImageManagementTarget:     1, // Always viable but lowest score
	}

	// Find the highest scoring strategy from preference order
	bestStrategy := ImageManagementTarget
	bestScore := 0

	for _, strategy := range preferenceOrder {
		if score := scores[strategy]; score > bestScore && td.isStrategyViable(strategy, availability) {
			bestStrategy = strategy
			bestScore = score
		}
	}

	log.Infof("Selected %s strategy (score: %d)", bestStrategy.String(), bestScore)
	return bestStrategy
}

// scoreLocalStrategy scores the local strategy based on available tools
func (td *ToolDetector) scoreLocalStrategy(availability *ToolAvailability) int {
	score := 0
	if availability.LocalDocker || availability.LocalPodman || availability.LocalNerdctl {
		score += 10 // Container runtime available
	}
	if availability.LocalHelm {
		score += 5 // Helm available
	}
	if availability.LocalKubeadm {
		score += 3 // Kubeadm available
	}
	if availability.LocalCurl {
		score += 1 // Curl available
	}
	return score
}

// scoreControllerStrategy scores the controller strategy based on available tools
func (td *ToolDetector) scoreControllerStrategy(availability *ToolAvailability) int {
	score := 0
	if availability.ControllerDocker || availability.ControllerPodman || availability.ControllerNerdctl {
		score += 8 // Container runtime available (slightly lower than local)
	}
	if availability.ControllerHelm {
		score += 4 // Helm available
	}
	if availability.ControllerKubeadm {
		score += 2 // Kubeadm available
	}
	if availability.ControllerCurl {
		score += 1 // Curl available
	}
	return score
}

// isStrategyViable checks if a strategy is viable given tool availability
func (td *ToolDetector) isStrategyViable(strategy ImageManagementStrategy, availability *ToolAvailability) bool {
	switch strategy {
	case ImageManagementLocal:
		// Need at least a container runtime locally
		return availability.LocalDocker || availability.LocalPodman || availability.LocalNerdctl
	case ImageManagementController:
		// Need at least a container runtime on controller
		return availability.ControllerDocker || availability.ControllerPodman || availability.ControllerNerdctl
	case ImageManagementTarget:
		// Always viable as we can install tools on target if needed
		return true
	default:
		return false
	}
}

// LogToolAvailability logs the detected tool availability for debugging
func (td *ToolDetector) LogToolAvailability(availability *ToolAvailability) {
	log.Infof("Tool availability detected:")
	log.Infof("  Local tools:")
	log.Infof("    Docker: %t, Podman: %t, Nerdctl: %t", availability.LocalDocker, availability.LocalPodman, availability.LocalNerdctl)
	log.Infof("    Helm: %t, Kubeadm: %t, Curl: %t", availability.LocalHelm, availability.LocalKubeadm, availability.LocalCurl)
	log.Infof("  Controller tools:")
	log.Infof("    Docker: %t, Podman: %t, Nerdctl: %t", availability.ControllerDocker, availability.ControllerPodman, availability.ControllerNerdctl)
	log.Infof("    Helm: %t, Kubeadm: %t, Curl: %t", availability.ControllerHelm, availability.ControllerKubeadm, availability.ControllerCurl)
}

// GetPreferredContainerTool returns the preferred container tool for a given strategy
func (td *ToolDetector) GetPreferredContainerTool(strategy ImageManagementStrategy, availability *ToolAvailability) string {
	switch strategy {
	case ImageManagementLocal:
		if availability.LocalNerdctl {
			return "nerdctl"
		}
		if availability.LocalDocker {
			return "docker"
		}
		if availability.LocalPodman {
			return "podman"
		}
	case ImageManagementController:
		if availability.ControllerNerdctl {
			return "nerdctl"
		}
		if availability.ControllerDocker {
			return "docker"
		}
		if availability.ControllerPodman {
			return "podman"
		}
	}
	return "nerdctl" // Default fallback
}
