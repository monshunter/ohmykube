package config

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestClusterConcurrentAccess tests concurrent access to cluster methods
func TestClusterConcurrentAccess(t *testing.T) {
	// Create a test cluster
	config := &Config{
		Name:              "test-cluster",
		KubernetesVersion: "v1.24.0",
		Provider:          "lima",
		ProxyMode:         "iptables",
		CNI:               "flannel",
		LB:                "none",
		CSI:               "none",
		Template:          "ubuntu-24.04",
		Master: Resource{
			CPU:    2,
			Memory: 4,
			Disk:   20,
		},
		Workers: []Resource{
			{CPU: 2, Memory: 4, Disk: 20},
			{CPU: 2, Memory: 4, Disk: 20},
		},
	}

	cluster := NewCluster(config)
	cluster.InitializeNodeGroupsFromSpec()

	// Number of concurrent goroutines
	numGoroutines := 10
	numOperations := 100

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numOperations)

	// Test concurrent read operations
	t.Run("ConcurrentReads", func(t *testing.T) {
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					// Test various read operations
					_ = cluster.GetKubernetesVersion()
					_ = cluster.GetProxyMode()
					_ = cluster.GetCNI()
					_ = cluster.GetTotalDesiredNodes()
					_ = cluster.GetTotalRunningNodes()
					_ = cluster.IsClusterFullyRunning()
					_ = cluster.GetMasterIP()
					_ = cluster.GetMasterName()
					_ = cluster.GetWorkerNames()
					_ = cluster.Nodes2IPsMap()
					_ = cluster.GetImageNames()
				}
			}(i)
		}
		wg.Wait()
	})

	// Test concurrent write operations
	t.Run("ConcurrentWrites", func(t *testing.T) {
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					nodeName := fmt.Sprintf("test-node-%d-%d", id, j)

					// Create node
					node := cluster.CreateNodeInGroup(2, nodeName)
					if node == nil {
						errors <- fmt.Errorf("failed to create node %s", nodeName)
						continue
					}

					// Set node properties
					cluster.SetNodeIP(nodeName, fmt.Sprintf("192.168.1.%d", (id*numOperations+j)%254+1))
					cluster.SetNodeHostname(nodeName, nodeName)
					cluster.SetNodeSystemInfo(nodeName, "Ubuntu 24.04", "6.8.0", "amd64", "linux")

					// Set node conditions
					cluster.SetNodeCondition(nodeName, ConditionTypeVMCreated, ConditionStatusTrue, "Created", "VM created successfully")
					cluster.SetNodeCondition(nodeName, ConditionTypeAuthInitialized, ConditionStatusTrue, "Initialized", "Auth initialized")

					// Set node phase
					cluster.SetPhaseForNode(nodeName, PhaseRunning)
				}
			}(i)
		}
		wg.Wait()
	})

	// Test concurrent mixed operations
	t.Run("ConcurrentMixed", func(t *testing.T) {
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					if j%2 == 0 {
						// Read operations
						_ = cluster.GetTotalRunningNodes()
						_ = cluster.HasCondition(ConditionTypeClusterReady, ConditionStatusTrue)
						node := cluster.GetNodeByName(fmt.Sprintf("test-node-%d-%d", id, j/2))
						if node != nil {
							_ = cluster.HasNodeCondition(node.Name, ConditionTypeVMCreated, ConditionStatusTrue)
						}
					} else {
						// Write operations
						cluster.SetCondition(ConditionTypeClusterReady, ConditionStatusFalse, "Testing", "Concurrent test")
						cluster.RecordImage(fmt.Sprintf("test-image-%d-%d", id, j))
						cluster.SetPhase(ClusterPhaseRunning)
					}
				}
			}(i)
		}
		wg.Wait()
	})

	// Check for errors
	close(errors)
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}

	// Verify final state
	if cluster.GetTotalRunningNodes() == 0 {
		t.Error("Expected some running nodes after concurrent operations")
	}

	t.Logf("Final cluster state: %d running nodes, %d total desired nodes",
		cluster.GetTotalRunningNodes(), cluster.GetTotalDesiredNodes())
}

// TestClusterRaceConditions tests for race conditions using go test -race
func TestClusterRaceConditions(t *testing.T) {
	cluster := &Cluster{
		ApiVersion: ApiVersion,
		Kind:       KindCluster,
		Metadata: Metadata{
			Name: "race-test-cluster",
		},
		Status: ClusterStatus{
			Phase: ClusterPhasePending,
			Nodes: []NodeGroupStatus{},
		},
	}

	// Simulate concurrent access that could cause race conditions
	var wg sync.WaitGroup
	numGoroutines := 5

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Concurrent node creation
			nodeName := fmt.Sprintf("race-node-%d", id)
			_ = cluster.CreateNodeInGroup(1, nodeName)

			// Concurrent property setting
			cluster.SetNodeIP(nodeName, fmt.Sprintf("10.0.0.%d", id+1))
			cluster.SetNodeCondition(nodeName, ConditionTypeVMCreated, ConditionStatusTrue, "Created", "Test")
			cluster.SetPhaseForNode(nodeName, PhaseRunning)

			// Concurrent reads
			_ = cluster.GetNodeByName(nodeName)
			_ = cluster.HasNodeCondition(nodeName, ConditionTypeVMCreated, ConditionStatusTrue)

			// Small delay to increase chance of race conditions
			time.Sleep(time.Millisecond)

			// More concurrent operations
			cluster.SetCondition(ConditionTypeClusterReady, ConditionStatusTrue, "Ready", "Test")
			_ = cluster.GetTotalRunningNodes()
		}(i)
	}

	wg.Wait()

	// Verify we have the expected number of nodes
	if len(cluster.Status.Nodes) == 0 {
		t.Error("Expected at least one node group after concurrent operations")
	}

	group := cluster.GetNodeGroupByID(1)
	if group == nil {
		t.Error("Expected to find node group 1")
	} else if len(group.Members) != numGoroutines {
		t.Errorf("Expected %d nodes, got %d", numGoroutines, len(group.Members))
	}
}
