package utils

import (
	"runtime"
	"testing"
)

func TestIsProcessRunning(t *testing.T) {
	// Test with a process that should always be running on Unix systems
	var testProcess string
	switch runtime.GOOS {
	case "darwin", "linux":
		testProcess = "kernel" // kernel processes should always be running
	case "windows":
		testProcess = "System" // System process should always be running on Windows
	default:
		t.Skipf("Unsupported OS: %s", runtime.GOOS)
	}

	// Test with a process that should exist
	running, err := IsProcessRunning(testProcess)
	if err != nil {
		t.Fatalf("IsProcessRunning failed: %v", err)
	}
	if !running {
		t.Errorf("Expected %s to be running, but it was not detected", testProcess)
	}

	// Test with a process that should not exist
	fakeProcess := "definitely_not_a_real_process_name_12345"
	running, err = IsProcessRunning(fakeProcess)
	if err != nil {
		t.Fatalf("IsProcessRunning failed for fake process: %v", err)
	}
	if running {
		t.Errorf("Expected %s to not be running, but it was detected as running", fakeProcess)
	}
}

func TestIsProcessRunning_SocketVmnet(t *testing.T) {
	// Test specifically for socket_vmnet (this may or may not be running)
	running, err := IsProcessRunning("socket_vmnet")
	if err != nil {
		t.Fatalf("IsProcessRunning failed for socket_vmnet: %v", err)
	}
	
	// We don't assert the result since socket_vmnet may or may not be running
	// This test just ensures the function doesn't crash
	t.Logf("socket_vmnet running: %v", running)
}
