package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/monshunter/ohmykube/pkg/controller"
	"github.com/monshunter/ohmykube/pkg/log"
)

// GracefulShutdownHandler handles graceful shutdown for commands that use a Manager
type GracefulShutdownHandler struct {
	ctx      context.Context
	cancel   context.CancelFunc
	manager  *controller.Manager
	exitFunc func(int) // Allow injection of exit function for testing
}

// NewGracefulShutdownHandler creates a new graceful shutdown handler
func NewGracefulShutdownHandler() *GracefulShutdownHandler {
	ctx, cancel := context.WithCancel(context.Background())

	handler := &GracefulShutdownHandler{
		ctx:      ctx,
		cancel:   cancel,
		exitFunc: os.Exit, // Default to os.Exit
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start signal handling goroutine
	go handler.handleSignals(sigChan)

	return handler
}

// SetManager sets the manager for cleanup
func (h *GracefulShutdownHandler) SetManager(manager *controller.Manager) {
	h.manager = manager
}

// Context returns the context that will be cancelled on shutdown
func (h *GracefulShutdownHandler) Context() context.Context {
	return h.ctx
}

// Close performs cleanup and cancels the context
func (h *GracefulShutdownHandler) Close() {
	h.cancel()
}

// SetExitFunc sets a custom exit function (useful for testing)
func (h *GracefulShutdownHandler) SetExitFunc(exitFunc func(int)) {
	h.exitFunc = exitFunc
}

// handleSignals handles OS signals for graceful shutdown
func (h *GracefulShutdownHandler) handleSignals(sigChan chan os.Signal) {
	sig := <-sigChan
	log.Infof("Received signal %v, initiating graceful shutdown...", sig)

	if h.manager != nil {
		log.Infof("Cleaning up resources...")
		if err := h.manager.Close(); err != nil {
			log.Errorf("Error during graceful shutdown: %v", err)
		}
	}

	h.cancel()
	h.exitFunc(0)
}
