package launcher

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/launcher/limactl"
	"github.com/monshunter/ohmykube/pkg/launcher/options"
)

// LauncherType represents the type of VM launcher to use
type LauncherType string

const (
	// LimactlLauncher is the limactl launcher
	LimactlLauncher LauncherType = "limactl"
	// Future cloud providers can be added here:
	// AliCloudLauncher LauncherType = "alicloud"
	// GKELauncher LauncherType = "gke"
	// AWSLauncher LauncherType = "aws"
	// TKELauncher LauncherType = "tke"
)

func (l LauncherType) String() string {
	return string(l)
}

func (l LauncherType) IsValid() bool {
	return l == LimactlLauncher
	// TODO: Add validation for future cloud providers
}

// NewLauncher creates a new launcher of the specified type
func NewLauncher(launcherType LauncherType, options *options.Options) (Launcher, error) {
	switch launcherType {
	case LimactlLauncher:
		return limactl.NewLimactlLauncher(options)
	default:
		return nil, fmt.Errorf("unsupported launcher type: %s, currently only 'limactl' is supported", launcherType)
	}
}
