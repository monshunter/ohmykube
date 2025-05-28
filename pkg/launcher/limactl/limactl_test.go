package limactl

import (
	"os"
	"strings"
	"testing"

	"github.com/monshunter/ohmykube/pkg/envar"
	"github.com/monshunter/ohmykube/pkg/launcher/options"
)

func TestCreateLimactlCommand(t *testing.T) {
	opt := &options.Options{
		Prefix:    "ohmykube",
		Template:  "ubuntu-24.04",
		Password:  "password",
		SSHKey:    "",
		SSHPubKey: "",
		Parallel:  1,
	}
	// Create a launcher instance
	launcher, err := NewLimactlLauncher(opt)
	if err != nil {
		t.Fatalf("Failed to create launcher: %v", err)
	}

	// Create a command
	cmd := launcher.createLimactlCommand("list")

	// Check that the command is correct (path might be full path to limactl)
	if !strings.HasSuffix(cmd.Path, "limactl") {
		t.Errorf("Expected command path to end with 'limactl', got '%s'", cmd.Path)
	}

	if len(cmd.Args) != 2 || cmd.Args[1] != "list" {
		t.Errorf("Expected args to be ['limactl', 'list'], got %v", cmd.Args)
	}

	// Check that LIMA_HOME environment variable is set
	expectedLimaHome := envar.OhMyKubeLimaHome()
	var limaHomeSet bool
	var actualLimaHome string

	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "LIMA_HOME=") {
			limaHomeSet = true
			actualLimaHome = strings.TrimPrefix(env, "LIMA_HOME=")
			break
		}
	}

	if !limaHomeSet {
		t.Error("LIMA_HOME environment variable is not set")
	}

	if actualLimaHome != expectedLimaHome {
		t.Errorf("Expected LIMA_HOME to be '%s', got '%s'", expectedLimaHome, actualLimaHome)
	}

	// Verify that other environment variables are preserved
	if len(cmd.Env) < len(os.Environ()) {
		t.Error("Expected all original environment variables to be preserved")
	}
}

func TestOhMyKubeLimaHome(t *testing.T) {
	limaHome := envar.OhMyKubeLimaHome()

	// Should contain .ohmykube/.lima
	if !strings.Contains(limaHome, ".ohmykube/.lima") {
		t.Errorf("Expected Lima home to contain '.ohmykube/.lima', got '%s'", limaHome)
	}

	// Should be an absolute path
	if !strings.HasPrefix(limaHome, "/") {
		t.Errorf("Expected Lima home to be an absolute path, got '%s'", limaHome)
	}
}
