package environment

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// ParallelBatchInitializer 并行批量初始化器
type ParallelBatchInitializer struct {
	sshRunner SSHCommandRunner
	nodeNames []string
	options   InitOptions
}

// NewParallelBatchInitializer 创建一个新的并行批量初始化器
func NewParallelBatchInitializer(sshRunner SSHCommandRunner, nodeNames []string) *ParallelBatchInitializer {
	return &ParallelBatchInitializer{
		sshRunner: sshRunner,
		nodeNames: nodeNames,
		options:   DefaultInitOptions(),
	}
}

// NewParallelBatchInitializerWithOptions 创建一个新的并行批量初始化器并指定选项
func NewParallelBatchInitializerWithOptions(sshRunner SSHCommandRunner, nodeNames []string, options InitOptions) *ParallelBatchInitializer {
	return &ParallelBatchInitializer{
		sshRunner: sshRunner,
		nodeNames: nodeNames,
		options:   options,
	}
}

// Initialize 并行初始化所有节点
func (b *ParallelBatchInitializer) Initialize() error {
	results := b.InitializeWithResults()
	return b.processResults(results)
}

// InitializeWithConcurrencyLimit 使用并发限制的并行初始化
func (b *ParallelBatchInitializer) InitializeWithConcurrencyLimit(maxConcurrency int) error {
	results := b.InitializeWithConcurrencyLimitAndResults(maxConcurrency)
	return b.processResults(results)
}

// InitializeWithResults 并行初始化所有节点并返回详细结果
func (b *ParallelBatchInitializer) InitializeWithResults() []NodeInitResult {
	var wg sync.WaitGroup
	resultChan := make(chan NodeInitResult, len(b.nodeNames))

	// 为每个节点启动一个goroutine执行初始化
	for _, nodeName := range b.nodeNames {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()

			// 为每个节点创建一个初始化器，并传递选项
			initializer := NewInitializerWithOptions(b.sshRunner, node, b.options)
			err := initializer.Initialize()

			resultChan <- NodeInitResult{
				NodeName: node,
				Success:  err == nil,
				Error:    err,
			}
		}(nodeName)
	}

	// 等待所有初始化完成
	wg.Wait()
	close(resultChan)

	// 收集所有结果
	results := make([]NodeInitResult, 0, len(b.nodeNames))
	for result := range resultChan {
		results = append(results, result)
	}

	return results
}

// InitializeWithConcurrencyLimitAndResults 使用并发限制的并行初始化并返回详细结果
func (b *ParallelBatchInitializer) InitializeWithConcurrencyLimitAndResults(maxConcurrency int) []NodeInitResult {
	if maxConcurrency <= 0 {
		maxConcurrency = len(b.nodeNames)
	}

	// 创建并发控制通道
	semaphore := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	resultChan := make(chan NodeInitResult, len(b.nodeNames))

	// 引入小的随机延迟，避免多个节点同时启动apt操作
	// 注意：Go 1.20+不再需要手动调用rand.Seed

	for _, nodeName := range b.nodeNames {
		wg.Add(1)
		go func(node string) {
			// 在启动初始化前随机等待一段时间，错开多个节点的启动时间
			randomDelay := time.Duration(rand.Intn(3000)) * time.Millisecond
			time.Sleep(randomDelay)

			// 获取并发槽
			semaphore <- struct{}{}
			defer func() {
				wg.Done()
				<-semaphore
			}()

			// 为节点创建初始化器并执行初始化，传递选项
			initializer := NewInitializerWithOptions(b.sshRunner, node, b.options)
			err := initializer.Initialize()

			resultChan <- NodeInitResult{
				NodeName: node,
				Success:  err == nil,
				Error:    err,
			}
		}(nodeName)
	}

	// 等待所有初始化完成
	wg.Wait()
	close(resultChan)

	// 收集所有结果
	results := make([]NodeInitResult, 0, len(b.nodeNames))
	for result := range resultChan {
		results = append(results, result)
	}

	return results
}

// processResults 处理初始化结果并生成适当的错误返回
func (b *ParallelBatchInitializer) processResults(results []NodeInitResult) error {
	failedNodes := []string{}
	errors := []string{}

	for _, result := range results {
		if !result.Success {
			failedNodes = append(failedNodes, result.NodeName)
			errors = append(errors, fmt.Sprintf("节点 %s: %v", result.NodeName, result.Error))
		}
	}

	if len(failedNodes) > 0 {
		return fmt.Errorf("以下节点初始化失败:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}
