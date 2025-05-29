package main

import (
	"time"

	"github.com/monshunter/ohmykube/pkg/log"
)

func main() {
	// Demo of the new progress system
	
	// 1. Demo: Simple progress bar
	log.Info("=== Simple Progress Bar Demo ===")
	pb := log.NewProgressBar("📦 Creating VMs", 100)
	for i := 0; i <= 100; i += 10 {
		pb.Update(i)
		time.Sleep(200 * time.Millisecond)
	}
	pb.Complete()
	
	time.Sleep(1 * time.Second)
	
	// 2. Demo: Multi-step progress (like cluster creation)
	log.Info("\n=== Multi-Step Progress Demo ===")
	progress := log.NewMultiStepProgress("Kubernetes cluster creation")
	
	// Add steps
	progress.AddStep("vm-creation", "Creating VMs")
	progress.AddStep("master-init", "Initializing master node")
	progress.AddStep("worker-join", "Joining worker nodes")
	progress.AddStep("cni-install", "Installing CNI (flannel)")
	progress.AddStep("csi-install", "Installing CSI (local-path)")
	
	// Simulate cluster creation process
	steps := []struct {
		name     string
		duration time.Duration
	}{
		{"Creating VMs", 3 * time.Second},
		{"Initializing master node", 2 * time.Second},
		{"Joining worker nodes", 2 * time.Second},
		{"Installing CNI (flannel)", 1 * time.Second},
		{"Installing CSI (local-path)", 1 * time.Second},
	}
	
	for i, step := range steps {
		progress.StartStep(i)
		time.Sleep(step.duration)
		progress.CompleteStep(i)
	}
	
	progress.Complete()
	
	// 3. Demo: Quiet mode
	log.Info("\n=== Quiet Mode Demo ===")
	log.SetQuiet(true)
	log.Info("This will not be shown in quiet mode")
	log.ProgressInfo("This will also not be shown in quiet mode")
	log.Error("But errors are still shown")
	
	// Reset to normal mode
	log.SetQuiet(false)
	log.Info("Back to normal mode")
	
	// 4. Demo: Verbose mode
	log.Info("\n=== Verbose Mode Demo ===")
	log.SetVerbose(true)
	log.Info("Normal info message")
	log.Debug("Debug message (only visible in verbose mode)")
	log.QuietInfo("This is shown in verbose mode")
	
	// Reset to normal mode
	log.SetVerbose(false)
	log.Debug("This debug message won't be shown")
	
	log.Info("\n=== Demo Complete ===")
	log.Info("Usage examples:")
	log.Info("  ohmykube up           # Default mode with progress bars")
	log.Info("  ohmykube up --verbose # Verbose mode with all details")
	log.Info("  ohmykube up --quiet   # Quiet mode with minimal output")
}
