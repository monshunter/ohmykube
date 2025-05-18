package launcher

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/launcher/limactl"
	"github.com/monshunter/ohmykube/pkg/launcher/multipass"
)

// LauncherType represents the type of VM launcher to use
type LauncherType string

const (
	// MultipassLauncher is the Multipass launcher
	MultipassLauncher LauncherType = "multipass"
	// LimactlLauncher is the limactl launcher
	LimactlLauncher LauncherType = "limactl"
)

func (l LauncherType) String() string {
	return string(l)
}

func (l LauncherType) IsValid() bool {
	return l == MultipassLauncher || l == LimactlLauncher
}

// Config contains common configuration options for launchers
type Config struct {
	Image     string
	Template  string
	Password  string
	SSHKey    string
	SSHPubKey string
	Parallel  int
}

// NewLauncher creates a new launcher of the specified type
func NewLauncher(launcherType LauncherType, config Config) (Launcher, error) {
	switch launcherType {
	case MultipassLauncher:
		return multipass.NewMultipassLauncher(config.Image, config.Password, config.SSHKey, config.SSHPubKey, config.Parallel)
	case LimactlLauncher:
		return limactl.NewLimactlLauncher(config.Template, config.Password, config.SSHKey, config.SSHPubKey, config.Parallel)
	default:
		return nil, fmt.Errorf("unsupported launcher type: %s", launcherType)
	}
}
