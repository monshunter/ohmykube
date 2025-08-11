package cache

import (
	"context"
	"strings"
	"testing"

	"github.com/monshunter/ohmykube/pkg/interfaces"
)

// TestMockSSHRunner for testing image discovery
type TestMockSSHRunner struct {
	commands      []string
	outputs       map[string]string
	defaultOutput string
}

func (m *TestMockSSHRunner) RunCommand(nodeName, command string) (string, error) {
	m.commands = append(m.commands, command)
	if output, exists := m.outputs[command]; exists {
		return output, nil
	}
	if m.defaultOutput != "" {
		return m.defaultOutput, nil
	}
	return "", nil
}

func (m *TestMockSSHRunner) UploadFile(nodeName, localPath, remotePath string) error {
	return nil
}

func (m *TestMockSSHRunner) DownloadFile(nodeName, remotePath, localPath string) error {
	return nil
}

func (m *TestMockSSHRunner) Close() error {
	return nil
}

func (m *TestMockSSHRunner) Address(host string) string {
	return host
}

func TestGetHelmChartImagesWithValuesFile(t *testing.T) {
	discovery := &DefaultImageDiscovery{}

	// Mock YAML output with some container images
	mockYAML := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  template:
    spec:
      containers:
      - name: app
        image: nginx:1.20
      - name: sidecar
        image: busybox:latest
`

	mockSSH := &TestMockSSHRunner{
		outputs:       make(map[string]string),
		defaultOutput: mockYAML,
	}

	// Test case 1: Helm chart with values file (remote path - should be used as-is)
	source := interfaces.ImageSource{
		Type:      "helm",
		ChartName: "test/chart",
		ChartRepo: "https://test.repo",
		Version:   "1.0.0",
		ChartValues: map[string]string{
			"image.tag": "v1.0.0",
		},
		ValuesFile: []string{"/remote/path/to/values.yaml"}, // Assume this exists on controller node
	}

	ctx := context.Background()
	_, err := discovery.getHelmChartImages(ctx, source, mockSSH, "controller")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check that the command was executed
	if len(mockSSH.commands) == 0 {
		t.Fatal("Expected at least one command to be executed")
	}

	command := mockSSH.commands[0]

	// Verify that the command includes values file arguments (should use the remote path as-is)
	if !strings.Contains(command, "--values /remote/path/to/values.yaml") {
		t.Errorf("Expected command to contain '--values /remote/path/to/values.yaml', but got: %s", command)
	}

	// Verify that the command includes set arguments
	if !strings.Contains(command, "--set image.tag=v1.0.0") {
		t.Errorf("Expected command to contain '--set image.tag=v1.0.0', but got: %s", command)
	}

	// Verify that the command includes version
	if !strings.Contains(command, "--version 1.0.0") {
		t.Errorf("Expected command to contain '--version 1.0.0', but got: %s", command)
	}
}

func TestGetHelmChartImagesWithMultipleValuesFiles(t *testing.T) {
	discovery := &DefaultImageDiscovery{}

	// Mock YAML output
	mockYAML := `
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - image: nginx:1.20
`

	mockSSH := &TestMockSSHRunner{
		outputs:       make(map[string]string),
		defaultOutput: mockYAML,
	}

	// Test case 2: Helm chart with multiple values files
	source := interfaces.ImageSource{
		Type:       "helm",
		ChartName:  "test/chart",
		ChartRepo:  "https://test.repo",
		Version:    "1.0.0",
		ValuesFile: []string{"/path/to/values1.yaml", "/path/to/values2.yaml"},
	}

	ctx := context.Background()
	_, err := discovery.getHelmChartImages(ctx, source, mockSSH, "controller")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	command := mockSSH.commands[0]

	// Verify that the command includes both values files
	if !strings.Contains(command, "--values /path/to/values1.yaml") {
		t.Errorf("Expected command to contain '--values /path/to/values1.yaml', but got: %s", command)
	}
	if !strings.Contains(command, "--values /path/to/values2.yaml") {
		t.Errorf("Expected command to contain '--values /path/to/values2.yaml', but got: %s", command)
	}
}

func TestGetHelmChartImagesWithoutValuesFile(t *testing.T) {
	discovery := &DefaultImageDiscovery{}

	// Mock YAML output
	mockYAML := `
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - image: nginx:1.20
`

	mockSSH := &TestMockSSHRunner{
		outputs:       make(map[string]string),
		defaultOutput: mockYAML,
	}

	// Test case 3: Helm chart without values file (should still work)
	source := interfaces.ImageSource{
		Type:      "helm",
		ChartName: "test/chart",
		ChartRepo: "https://test.repo",
		Version:   "1.0.0",
		ChartValues: map[string]string{
			"image.tag": "v1.0.0",
		},
		// ValuesFile is empty
	}

	ctx := context.Background()
	_, err := discovery.getHelmChartImages(ctx, source, mockSSH, "controller")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	command := mockSSH.commands[0]

	// Verify that the command includes set arguments but no values file
	if !strings.Contains(command, "--set image.tag=v1.0.0") {
		t.Errorf("Expected command to contain '--set image.tag=v1.0.0', but got: %s", command)
	}

	// Should not contain --values since no values file is specified
	if strings.Contains(command, "--values") {
		t.Errorf("Expected command to not contain '--values' when no values file is specified, but got: %s", command)
	}
}

func TestGetHelmChartImagesWithURLValuesFile(t *testing.T) {
	discovery := &DefaultImageDiscovery{}

	// Mock YAML output
	mockYAML := `
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - image: nginx:1.20
`

	mockSSH := &TestMockSSHRunner{
		outputs:       make(map[string]string),
		defaultOutput: mockYAML,
	}

	// Test case: Helm chart with URL values file
	source := interfaces.ImageSource{
		Type:       "helm",
		ChartName:  "test/chart",
		ChartRepo:  "https://test.repo",
		Version:    "1.0.0",
		ValuesFile: []string{"https://example.com/values.yaml"},
	}

	ctx := context.Background()
	_, err := discovery.getHelmChartImages(ctx, source, mockSSH, "controller")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check that the command was executed
	if len(mockSSH.commands) == 0 {
		t.Fatal("Expected at least one command to be executed")
	}

	// Should have at least 2 commands: curl download + helm template
	if len(mockSSH.commands) < 2 {
		t.Fatalf("Expected at least 2 commands (curl + helm), got %d", len(mockSSH.commands))
	}

	// Check for curl command
	curlFound := false
	helmCommand := ""
	for _, cmd := range mockSSH.commands {
		if strings.Contains(cmd, "curl") && strings.Contains(cmd, "https://example.com/values.yaml") {
			curlFound = true
		}
		if strings.Contains(cmd, "helm template") {
			helmCommand = cmd
		}
	}

	if !curlFound {
		t.Errorf("Expected to find curl command for downloading values file, commands: %v", mockSSH.commands)
	}

	// Verify that the helm command uses the downloaded file path
	if helmCommand != "" && !strings.Contains(helmCommand, "--values /tmp/helm-discovery-values-0.yaml") {
		t.Errorf("Expected helm command to contain '--values /tmp/helm-discovery-values-0.yaml', but got: %s", helmCommand)
	}
}
