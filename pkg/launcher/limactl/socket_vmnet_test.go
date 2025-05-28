package limactl

import (
	"runtime"
	"strings"
	"testing"
)

func TestCheckSocketVmnetDaemon(t *testing.T) {
	err := checkSocketVmnetDaemon()

	if runtime.GOOS != "darwin" {
		// On non-macOS systems, the function should return nil (no error)
		if err != nil {
			t.Errorf("Expected no error on %s, but got: %v", runtime.GOOS, err)
		}
		return
	}

	// On macOS, the function should either succeed (if socket_vmnet is running)
	// or fail with a specific error message (if socket_vmnet is not running)
	if err != nil {
		// Check if the error message contains expected text
		expectedText := "socket_vmnet daemon is not running"
		if !strings.Contains(err.Error(), expectedText) {
			t.Errorf("Expected error to contain '%s', but got: %v", expectedText, err)
		}
	}
	// If err is nil, socket_vmnet is running, which is fine
}
