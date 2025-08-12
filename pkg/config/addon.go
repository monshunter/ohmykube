package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// AddonSpec defines the configuration for installing an addon
type AddonSpec struct {
	// Basic required fields (minimum needed for command line JSON)
	Name    string `json:"name" yaml:"name"`       // Addon name
	Type    string `json:"type" yaml:"type"`       // "helm" or "manifest"
	Version string `json:"version" yaml:"version"` // Version information
	Enabled *bool  `json:"enabled" yaml:"enabled"` // Whether enabled (default true)

	// Manifest type fields (supports local files, URLs, or remote paths)
	Files []string `json:"files,omitempty" yaml:"files,omitempty"` // Manifest file paths/URLs

	// Helm type fields
	Repo        string            `json:"repo,omitempty" yaml:"repo,omitempty"`               // Helm repository URL
	Chart       string            `json:"chart,omitempty" yaml:"chart,omitempty"`             // Helm chart name
	Values      map[string]string `json:"values,omitempty" yaml:"values,omitempty"`           // Helm values overrides
	ValuesFiles []string          `json:"valuesFiles,omitempty" yaml:"valuesFiles,omitempty"` // Values file paths (supports multiple files)

	// Advanced configuration fields (full customization options in config files)
	Namespace    string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`       // Target namespace
	Priority     int               `json:"priority,omitempty" yaml:"priority,omitempty"`         // Installation priority (lower number = higher priority)
	Dependencies []string          `json:"dependencies,omitempty" yaml:"dependencies,omitempty"` // Dependencies on other addons
	Timeout      string            `json:"timeout,omitempty" yaml:"timeout,omitempty"`           // Installation timeout (default 300s)
	Labels       map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`             // Addon labels
	Annotations  map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`   // Addon annotations

	// Custom installation configuration
	PreInstall  []string `json:"preInstall,omitempty" yaml:"preInstall,omitempty"`   // Commands to run before installation
	PostInstall []string `json:"postInstall,omitempty" yaml:"postInstall,omitempty"` // Commands to run after installation
}

// ValidateMinimal performs minimal validation (for command line JSON)
func (a *AddonSpec) ValidateMinimal() error {
	if a.Name == "" || a.Type == "" || a.Version == "" {
		return fmt.Errorf("name, type and version are required")
	}

	switch a.Type {
	case "helm":
		if a.Chart == "" {
			return fmt.Errorf("chart is required for helm type addon")
		}
		// repo is only required for remote charts (not local charts)
		if !a.IsLocalChart() && a.Repo == "" {
			return fmt.Errorf("repo is required for remote helm charts")
		}
	case "manifest":
		if len(a.Files) == 0 {
			return fmt.Errorf("files are required for manifest type addon")
		}
	default:
		return fmt.Errorf("unsupported addon type: %s (supported: helm, manifest)", a.Type)
	}
	return nil
}

// Validate performs full validation (for config files)
func (a *AddonSpec) Validate() error {
	if err := a.ValidateMinimal(); err != nil {
		return err
	}

	// Set default values
	if a.Enabled == nil {
		a.Enabled = new(bool)
		*a.Enabled = true
	}
	if a.Timeout == "" {
		a.Timeout = "300s"
	}
	if a.Priority == 0 {
		a.Priority = 100
	}

	return nil
}

// IsEnabled returns whether the addon is enabled
func (a *AddonSpec) IsEnabled() bool {
	if a.Enabled == nil {
		return true // default enabled
	}
	return *a.Enabled
}

// IsLocalChart determines if the chart field refers to a local chart path
func (a *AddonSpec) IsLocalChart() bool {
	if a.Type != "helm" || a.Chart == "" {
		return false
	}

	// Check if chart path starts with / or ./ or ../ (Unix paths)
	if strings.HasPrefix(a.Chart, "/") || strings.HasPrefix(a.Chart, "./") || strings.HasPrefix(a.Chart, "../") {
		return true
	}

	// Check if chart path contains : (Windows drive letter like C:)
	if strings.Contains(a.Chart, ":") && !strings.HasPrefix(a.Chart, "http") {
		return true
	}

	// Check if it's a relative path that exists locally
	if _, err := os.Stat(a.Chart); err == nil {
		return true
	}

	return false
}

// Equals compares two AddonSpec for content equality (excluding metadata like labels/annotations)
func (a *AddonSpec) Equals(other *AddonSpec) bool {
	if a.Name != other.Name || a.Type != other.Type || a.Version != other.Version {
		return false
	}

	if a.IsEnabled() != other.IsEnabled() {
		return false
	}

	// Compare type-specific fields
	switch a.Type {
	case "helm":
		if a.Repo != other.Repo || a.Chart != other.Chart {
			return false
		}
		// Compare ValuesFiles slices
		if !stringSliceEquals(a.ValuesFiles, other.ValuesFiles) {
			return false
		}
		// Compare values maps
		if !mapStringEquals(a.Values, other.Values) {
			return false
		}
	case "manifest":
		// Compare files slices
		if !stringSliceEquals(a.Files, other.Files) {
			return false
		}
	}

	// Compare advanced configuration
	if a.Namespace != other.Namespace || a.Priority != other.Priority || a.Timeout != other.Timeout {
		return false
	}

	// Compare dependencies
	if !stringSliceEquals(a.Dependencies, other.Dependencies) {
		return false
	}

	// Compare pre/post install commands
	if !stringSliceEquals(a.PreInstall, other.PreInstall) || !stringSliceEquals(a.PostInstall, other.PostInstall) {
		return false
	}

	return true
}

// HasMajorChanges checks if the change requires reinstallation vs simple update
func (a *AddonSpec) HasMajorChanges(other *AddonSpec) bool {
	// Type change always requires reinstallation
	if a.Type != other.Type {
		return true
	}

	// Namespace change requires reinstallation
	if a.Namespace != other.Namespace {
		return true
	}

	// For helm, changing repo or chart requires reinstallation
	if a.Type == "helm" && (a.Repo != other.Repo || a.Chart != other.Chart) {
		return true
	}

	// For manifest, changing URL requires reinstallation
	if a.Type == "manifest" && !stringSliceEquals(a.Files, other.Files) {
		return true
	}

	return false
}

// mapStringEquals compares two string maps for equality
func mapStringEquals(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, exists := b[k]; !exists || v != bv {
			return false
		}
	}
	return true
}

// stringSliceEquals compares two string slices for equality
func stringSliceEquals(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

// AddonPhase defines the lifecycle states of an addon
type AddonPhase string

const (
	AddonPhasePending    AddonPhase = "Pending"    // Waiting for installation
	AddonPhaseInstalling AddonPhase = "Installing" // Currently installing
	AddonPhaseInstalled  AddonPhase = "Installed"  // Successfully installed
	AddonPhaseUpgrading  AddonPhase = "Upgrading"  // Currently upgrading
	AddonPhaseUpgraded   AddonPhase = "Upgraded"   // Successfully upgraded
	AddonPhaseFailed     AddonPhase = "Failed"     // Installation/upgrade failed
	AddonPhaseRemoving   AddonPhase = "Removing"   // Currently being removed
	AddonPhaseRemoved    AddonPhase = "Removed"    // Successfully removed
	AddonPhaseReady      AddonPhase = "Ready"      // Installation completed and ready
	AddonPhaseUnknown    AddonPhase = "Unknown"    // Status unknown
)

// AddonOperation defines the type of operation to perform on an addon
type AddonOperation string

const (
	AddonOperationInstall  AddonOperation = "Install"  // Install new addon
	AddonOperationUpdate   AddonOperation = "Update"   // Update existing addon
	AddonOperationRemove   AddonOperation = "Remove"   // Remove addon (when disabled)
	AddonOperationNoChange AddonOperation = "NoChange" // Keep current state (no changes detected)
)

// AddonTask describes a specific operation to be performed on an addon
type AddonTask struct {
	Name          string         `json:"name"`
	Operation     AddonOperation `json:"operation"`               // Install, Update, Remove, NoChange
	Spec          AddonSpec      `json:"spec"`                    // Current desired spec
	CurrentStatus *AddonStatus   `json:"currentStatus,omitempty"` // Current installation status
	Reason        string         `json:"reason"`                  // Why this operation is needed
	Priority      int            `json:"priority"`                // Task priority (from spec)
}

// AddonResource tracks K8s resources created by an addon
type AddonResource struct {
	APIVersion string `yaml:"apiVersion,omitempty"`
	Kind       string `yaml:"kind,omitempty"`
	Name       string `yaml:"name,omitempty"`
	Namespace  string `yaml:"namespace,omitempty"`
	UID        string `yaml:"uid,omitempty"`     // K8s resource UID
	Created    bool   `yaml:"created,omitempty"` // Whether created
}

// AddonStatus represents the runtime status of an addon
type AddonStatus struct {
	Name             string          `yaml:"name,omitempty"`
	Phase            AddonPhase      `yaml:"phase,omitempty"`
	InstalledVersion string          `yaml:"installedVersion,omitempty"`
	DesiredVersion   string          `yaml:"desiredVersion,omitempty"`
	Namespace        string          `yaml:"namespace,omitempty"`
	InstallTime      *time.Time      `yaml:"installTime,omitempty"`
	LastUpdateTime   *time.Time      `yaml:"lastUpdateTime,omitempty"`
	Conditions       []Condition     `yaml:"conditions,omitempty"`
	Resources        []AddonResource `yaml:"resources,omitempty"` // Tracked K8s resources
	Images           []string        `yaml:"images,omitempty"`    // Cached image list
	Message          string          `yaml:"message,omitempty"`   // Status message
	Reason           string          `yaml:"reason,omitempty"`    // Status reason
}
