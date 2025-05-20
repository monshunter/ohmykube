package initializer

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// ParallelBatchInitializer parallel batch initializer
type ParallelBatchInitializer struct {
	sshRunner SSHCommandRunner
	nodeNames []string
	options   InitOptions
}

// NewParallelBatchInitializer creates a new parallel batch initializer
func NewParallelBatchInitializer(sshRunner SSHCommandRunner, nodeNames []string) *ParallelBatchInitializer {
	return &ParallelBatchInitializer{
		sshRunner: sshRunner,
		nodeNames: nodeNames,
		options:   DefaultInitOptions(),
	}
}

// NewParallelBatchInitializerWithOptions creates a new parallel batch initializer with specified options
func NewParallelBatchInitializerWithOptions(sshRunner SSHCommandRunner, nodeNames []string, options InitOptions) *ParallelBatchInitializer {
	return &ParallelBatchInitializer{
		sshRunner: sshRunner,
		nodeNames: nodeNames,
		options:   options,
	}
}

// Initialize initializes all nodes in parallel
func (b *ParallelBatchInitializer) Initialize() error {
	results := b.InitializeWithResults()
	return b.processResults(results)
}

// InitializeWithConcurrencyLimit initializes with concurrency limit
func (b *ParallelBatchInitializer) InitializeWithConcurrencyLimit(maxConcurrency int) error {
	results := b.InitializeWithConcurrencyLimitAndResults(maxConcurrency)
	return b.processResults(results)
}

// InitializeWithResults initializes all nodes in parallel and returns detailed results
func (b *ParallelBatchInitializer) InitializeWithResults() []NodeInitResult {
	var wg sync.WaitGroup
	resultChan := make(chan NodeInitResult, len(b.nodeNames))

	// Start a goroutine for each node to perform initialization
	for _, nodeName := range b.nodeNames {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()

			// Create an initializer for each node, and pass options
			initializer := NewInitializerWithOptions(b.sshRunner, node, b.options)
			err := initializer.Initialize()

			resultChan <- NodeInitResult{
				NodeName: node,
				Success:  err == nil,
				Error:    err,
			}
		}(nodeName)
	}

	// Wait for all initializations to complete
	wg.Wait()
	close(resultChan)

	// Collect all results
	results := make([]NodeInitResult, 0, len(b.nodeNames))
	for result := range resultChan {
		results = append(results, result)
	}

	return results
}

// InitializeWithConcurrencyLimitAndResults initializes with concurrency limit and returns detailed results
func (b *ParallelBatchInitializer) InitializeWithConcurrencyLimitAndResults(maxConcurrency int) []NodeInitResult {
	if maxConcurrency <= 0 {
		maxConcurrency = len(b.nodeNames)
	}

	// Create concurrency control channel
	semaphore := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	resultChan := make(chan NodeInitResult, len(b.nodeNames))

	// Introduce small random delays to avoid multiple nodes starting apt operations simultaneously
	// Note: Go 1.20+ no longer requires manual calls to rand.Seed

	for _, nodeName := range b.nodeNames {
		wg.Add(1)
		go func(node string) {
			// Wait a random time before starting initialization to stagger node start times
			randomDelay := time.Duration(rand.Intn(3000)) * time.Millisecond
			time.Sleep(randomDelay)

			// Acquire concurrency slot
			semaphore <- struct{}{}
			defer func() {
				wg.Done()
				<-semaphore
			}()

			// Create initializer for node and perform initialization, passing options
			initializer := NewInitializerWithOptions(b.sshRunner, node, b.options)
			err := initializer.Initialize()

			resultChan <- NodeInitResult{
				NodeName: node,
				Success:  err == nil,
				Error:    err,
			}
		}(nodeName)
	}

	// Wait for all initializations to complete
	wg.Wait()
	close(resultChan)

	// Collect all results
	results := make([]NodeInitResult, 0, len(b.nodeNames))
	for result := range resultChan {
		results = append(results, result)
	}

	return results
}

// processResults processes initialization results and generates appropriate error return
func (b *ParallelBatchInitializer) processResults(results []NodeInitResult) error {
	failedNodes := []string{}
	errors := []string{}

	for _, result := range results {
		if !result.Success {
			failedNodes = append(failedNodes, result.NodeName)
			errors = append(errors, fmt.Sprintf("Node %s: %v", result.NodeName, result.Error))
		}
	}

	if len(failedNodes) > 0 {
		return fmt.Errorf("The following nodes failed to initialize:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}
