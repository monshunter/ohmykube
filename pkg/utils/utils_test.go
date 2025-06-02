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

func TestNormalizeK8sVersion(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "Add v prefix and patch version",
			input:       "1.24",
			expected:    "v1.24.0",
			expectError: false,
		},
		{
			name:        "Add v prefix only",
			input:       "1.24.1",
			expected:    "v1.24.1",
			expectError: false,
		},
		{
			name:        "Add patch version only",
			input:       "v1.24",
			expected:    "v1.24.0",
			expectError: false,
		},
		{
			name:        "Already normalized",
			input:       "v1.24.1",
			expected:    "v1.24.1",
			expectError: false,
		},
		{
			name:        "With whitespace",
			input:       "  v1.24.1  ",
			expected:    "v1.24.1",
			expectError: false,
		},
		{
			name:        "Higher version numbers",
			input:       "1.33",
			expected:    "v1.33.0",
			expectError: false,
		},
		{
			name:        "Empty string",
			input:       "",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Invalid format - only major",
			input:       "1",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Invalid format - non-numeric",
			input:       "v1.24.abc",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Invalid format - missing minor",
			input:       "v1.",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Invalid format - extra parts",
			input:       "v1.24.1.2",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeK8sVersion(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for input '%s', but got none", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for input '%s': %v", tt.input, err)
				return
			}

			if result != tt.expected {
				t.Errorf("For input '%s', expected '%s', but got '%s'", tt.input, tt.expected, result)
			}
		})
	}
}

func TestNormalizeK8sVersion_EdgeCases(t *testing.T) {
	// Test with realistic Kubernetes versions
	realVersions := []struct {
		input    string
		expected string
	}{
		{"1.28", "v1.28.0"},
		{"1.29.1", "v1.29.1"},
		{"v1.30.0", "v1.30.0"},
		{"1.31.2", "v1.31.2"},
		{"v1.32", "v1.32.0"},
		{"1.33.0", "v1.33.0"},
	}

	for _, tt := range realVersions {
		t.Run("Real_version_"+tt.input, func(t *testing.T) {
			result, err := NormalizeK8sVersion(tt.input)
			if err != nil {
				t.Errorf("Unexpected error for realistic version '%s': %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("For realistic version '%s', expected '%s', but got '%s'", tt.input, tt.expected, result)
			}
		})
	}
}
