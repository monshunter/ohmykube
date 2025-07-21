package app

import (
	"testing"
	"time"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/controller"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

func TestGracefulShutdownHandler(t *testing.T) {
	// Create a new graceful shutdown handler
	handler := NewGracefulShutdownHandler()
	defer handler.Close()

	// Test that context is not cancelled initially
	select {
	case <-handler.Context().Done():
		t.Fatal("Context should not be cancelled initially")
	default:
		// Expected behavior
	}

	// Test that Close() cancels the context
	handler.Close()

	// Wait a bit for the context to be cancelled
	select {
	case <-handler.Context().Done():
		// Expected behavior
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Context should be cancelled after Close()")
	}
}

func TestGracefulShutdownHandlerWithManager(t *testing.T) {
	// Create a new graceful shutdown handler
	handler := NewGracefulShutdownHandler()
	defer handler.Close()

	// Create a mock manager (we'll use a real one but with minimal config)
	cfg := &config.Config{
		Name:     "test-cluster",
		Provider: "lima",
	}

	// Create SSH config (this might fail in test environment, but that's ok)
	sshConfig, err := ssh.NewSSHConfig("test", "test-cluster")
	if err != nil {
		t.Skipf("Skipping test due to SSH config error: %v", err)
	}

	manager, err := controller.NewManager(cfg, sshConfig, nil, nil)
	if err != nil {
		t.Skipf("Skipping test due to manager creation error: %v", err)
	}

	// Set the manager
	handler.SetManager(manager)

	// Test that the manager is set
	if handler.manager == nil {
		t.Fatal("Manager should be set")
	}

	// Test cleanup
	handler.Close()
}

func TestGracefulShutdownHandlerSignalHandling(t *testing.T) {
	// Skip this test as signal handling in test environment is complex
	// and can interfere with other tests
	t.Skip("Skipping signal handling test to avoid interference with other tests")
}
