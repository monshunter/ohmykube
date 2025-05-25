package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// MockSSHRunner implements the SSHCommandRunner interface for testing
type MockSSHRunner struct {
	commands []string
	outputs  map[string]string
}

func NewMockSSHRunner() *MockSSHRunner {
	return &MockSSHRunner{
		commands: make([]string, 0),
		outputs:  make(map[string]string),
	}
}

func (m *MockSSHRunner) RunSSHCommand(nodeName string, command string) (string, error) {
	m.commands = append(m.commands, command)
	if output, exists := m.outputs[command]; exists {
		return output, nil
	}
	return "success", nil
}

func (m *MockSSHRunner) SetOutput(command, output string) {
	m.outputs[command] = output
}

func TestNewPackageManager(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "ohmykube-cache-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set environment variable to use temp directory
	os.Setenv("OMYKUBE_HOME", tempDir)
	defer os.Unsetenv("OMYKUBE_HOME")

	// Create manager
	manager, err := NewPackageManager()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Check if cache directory was created
	cacheDir := filepath.Join(tempDir, "cache", "packages")
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Errorf("Cache directory was not created: %s", cacheDir)
	}

	// Check if index file was created
	indexPath := filepath.Join(cacheDir, "index.yaml")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Errorf("Index file was not created: %s", indexPath)
	}
}

func TestPackageDefinitions(t *testing.T) {
	definitions := GetPackageDefinitions()

	// Test that all expected packages are defined
	expectedPackages := []string{
		"containerd", "runc", "cni-plugins", "kubectl",
		"kubeadm", "kubelet", "helm", "crictl", "nerdctl",
	}

	for _, pkg := range expectedPackages {
		if _, exists := definitions[pkg]; !exists {
			t.Errorf("Package definition not found: %s", pkg)
		}
	}

	// Test containerd package definition
	containerd := definitions["containerd"]
	if containerd.Name != "containerd" {
		t.Errorf("Expected containerd name, got %s", containerd.Name)
	}

	// Test URL generation
	url := containerd.GetURL("2.1.0", "amd64")
	expectedURL := "https://github.com/containerd/containerd/releases/download/v2.1.0/containerd-2.1.0-linux-amd64.tar.gz"
	if url != expectedURL {
		t.Errorf("Expected URL %s, got %s", expectedURL, url)
	}

	// Test filename generation
	filename := containerd.GetFilename("2.1.0", "amd64")
	expectedFilename := "containerd-2.1.0-linux-amd64.tar.gz"
	if filename != expectedFilename {
		t.Errorf("Expected filename %s, got %s", expectedFilename, filename)
	}
}

func TestPackageKey(t *testing.T) {
	key := PackageKey("containerd", "2.1.0", "amd64")
	expected := "containerd-2.1.0-amd64"
	if key != expected {
		t.Errorf("Expected key %s, got %s", expected, key)
	}
}

func TestPackageInfo_GetRemotePath(t *testing.T) {
	packageInfo := PackageInfo{
		Name:         "containerd",
		Version:      "2.1.0",
		Architecture: "amd64",
	}

	remotePath := packageInfo.GetRemotePath()
	expected := "/usr/local/src/containerd/containerd-2.1.0-amd64.tar.zst"
	if remotePath != expected {
		t.Errorf("Expected remote path %s, got %s", expected, remotePath)
	}

	// Test with custom remote path
	packageInfo.RemotePath = "/custom/path/package.tar.zst"
	remotePath = packageInfo.GetRemotePath()
	if remotePath != "/custom/path/package.tar.zst" {
		t.Errorf("Expected custom remote path, got %s", remotePath)
	}
}

func TestIsPackageCached(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "ohmykube-cache-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set environment variable to use temp directory
	os.Setenv("OMYKUBE_HOME", tempDir)
	defer os.Unsetenv("OMYKUBE_HOME")

	// Create manager
	manager, err := NewPackageManager()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Test non-existent package
	if manager.IsPackageCached("nonexistent", "1.0.0", "amd64") {
		t.Error("Expected package to not be cached")
	}

	// Add a package to the index
	packageInfo := PackageInfo{
		Name:           "test-package",
		Version:        "1.0.0",
		Architecture:   "amd64",
		LocalPath:      filepath.Join(tempDir, "test-package.tar.zst"),
		CachedAt:       time.Now(),
		LastAccessedAt: time.Now(),
	}

	// Create the file
	if err := os.WriteFile(packageInfo.LocalPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	manager.addPackage(packageInfo)

	// Test existing package
	if !manager.IsPackageCached("test-package", "1.0.0", "amd64") {
		t.Error("Expected package to be cached")
	}
}

func TestGetCacheStats(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "ohmykube-cache-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set environment variable to use temp directory
	os.Setenv("OMYKUBE_HOME", tempDir)
	defer os.Unsetenv("OMYKUBE_HOME")

	// Create manager
	manager, err := NewPackageManager()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Add some test packages
	packages := []PackageInfo{
		{
			Name:           "package1",
			Version:        "1.0.0",
			Architecture:   "amd64",
			CompressedSize: 1000,
			OriginalSize:   2000,
		},
		{
			Name:           "package2",
			Version:        "2.0.0",
			Architecture:   "arm64",
			CompressedSize: 1500,
			OriginalSize:   3000,
		},
	}

	for _, pkg := range packages {
		manager.addPackage(pkg)
	}

	// Test stats
	totalPackages, totalCompressed, totalOriginal := manager.GetCacheStats()

	if totalPackages != 2 {
		t.Errorf("Expected 2 packages, got %d", totalPackages)
	}

	if totalCompressed != 2500 {
		t.Errorf("Expected total compressed size 2500, got %d", totalCompressed)
	}

	if totalOriginal != 5000 {
		t.Errorf("Expected total original size 5000, got %d", totalOriginal)
	}
}

func TestNormalizeToTarZst(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "ohmykube-normalize-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create compressor
	compressor, err := NewCompressor()
	if err != nil {
		t.Fatalf("Failed to create compressor: %v", err)
	}
	defer compressor.Close()

	// Test Case 1: Binary file normalization
	t.Run("Binary file", func(t *testing.T) {
		// Create a test binary file
		binaryPath := filepath.Join(tempDir, "test-binary")
		binaryContent := []byte("#!/bin/bash\necho 'Hello World'\n")
		if err := os.WriteFile(binaryPath, binaryContent, 0755); err != nil {
			t.Fatalf("Failed to create test binary: %v", err)
		}

		// Normalize to tar.zst
		outputPath := filepath.Join(tempDir, "test-binary.tar.zst")
		if err := compressor.NormalizeToTarZst(binaryPath, outputPath, FileFormatBinary); err != nil {
			t.Fatalf("Failed to normalize binary file: %v", err)
		}

		// Verify output file exists
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			t.Error("Output file was not created")
		}

		// Verify it's a valid tar.zst file by trying to decompress it
		extractDir := filepath.Join(tempDir, "extract-binary")
		if err := compressor.DecompressFile(outputPath, extractDir); err != nil {
			t.Errorf("Failed to decompress normalized file: %v", err)
		}

		// Verify the extracted file content
		extractedFile := filepath.Join(extractDir, "test-binary")
		extractedContent, err := os.ReadFile(extractedFile)
		if err != nil {
			t.Errorf("Failed to read extracted file: %v", err)
		} else if string(extractedContent) != string(binaryContent) {
			t.Errorf("Extracted content doesn't match original. Expected: %s, Got: %s",
				string(binaryContent), string(extractedContent))
		}
	})

	// Test Case 2: Test format detection
	t.Run("Format detection", func(t *testing.T) {
		testCases := []struct {
			format   FileFormat
			expected string
		}{
			{FileFormatBinary, "binary"},
			{FileFormatTar, "tar"},
			{FileFormatTarGz, "tar.gz"},
			{FileFormatTgz, "tgz"},
			{FileFormatTarBz2, "tar.bz2"},
			{FileFormatTarXz, "tar.xz"},
		}

		for _, tc := range testCases {
			if string(tc.format) != tc.expected {
				t.Errorf("Format constant mismatch: expected %s, got %s", tc.expected, string(tc.format))
			}
		}
	})
}
