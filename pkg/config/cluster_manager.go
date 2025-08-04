package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SetCurrentCluster sets the current cluster by saving it to a config file
func SetCurrentCluster(clusterName string) error {
	ohmykubeDir, err := OhMyKubeDir()
	if err != nil {
		return err
	}

	currentClusterFile := filepath.Join(ohmykubeDir, "current-cluster")
	return os.WriteFile(currentClusterFile, []byte(clusterName), 0644)
}

// GetCurrentCluster gets the current cluster name from config file
func GetCurrentCluster() (string, error) {
	ohmykubeDir, err := OhMyKubeDir()
	if err != nil {
		return "", err
	}

	currentClusterFile := filepath.Join(ohmykubeDir, "current-cluster")
	data, err := os.ReadFile(currentClusterFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "ohmykube", nil // default cluster name
		}
		return "", err
	}

	clusterName := strings.TrimSpace(string(data))
	if clusterName == "" {
		return "ohmykube", nil // default cluster name if file is empty
	}
	return clusterName, nil
}

// ListAllClusters discovers all clusters by scanning the .ohmykube directory
func ListAllClusters() ([]string, error) {
	ohmykubeDir, err := OhMyKubeDir()
	if err != nil {
		return nil, err
	}

	// Check if .ohmykube directory exists
	if _, err := os.Stat(ohmykubeDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(ohmykubeDir)
	if err != nil {
		return nil, err
	}

	var clusters []string
	for _, entry := range entries {
		if entry.IsDir() {
			clusterYaml := filepath.Join(ohmykubeDir, entry.Name(), "cluster.yaml")
			if _, err := os.Stat(clusterYaml); err == nil {
				clusters = append(clusters, entry.Name())
			}
		}
	}

	return clusters, nil
}

// calculateUptime calculates the cluster uptime based on creation annotation
func calculateUptime(cluster *Cluster) string {
	if cluster.Status.Phase != ClusterPhaseRunning {
		return "-"
	}

	createdAt, exists := cluster.Metadata.Annotations["ohmykube.dev/created-at"]
	if !exists {
		return "unknown"
	}

	createdTime, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return "unknown"
	}

	uptime := time.Since(createdTime) / time.Second * time.Second
	// return fmt.Sprintf("%s", uptime.String())

	// Format uptime in a human-readable way
	if uptime < time.Hour {
		return fmt.Sprintf("%dmin", int(uptime.Minutes()))
	} else if uptime < 24*time.Hour {
		hours := int(uptime.Hours())
		minutes := int(uptime.Minutes()) % 60
		if minutes > 0 {
			return fmt.Sprintf("%dh%dmin", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	} else {
		days := int(uptime.Hours()) / 24
		hours := int(uptime.Hours()) % 24
		if hours > 0 {
			return fmt.Sprintf("%dd%dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}
}

type ClusterInfo struct {
	Name              string
	Status            ClusterPhase
	KubernetesVersion string
	Nodes             int
	RunningNodes      int
	CNI               string
	CSI               string
	LoadBalancer      string
	FilePath          string
	Uptime            string
}

// GetClusterInfo returns detailed information about a cluster
func GetClusterInfo(clusterName string) (*ClusterInfo, error) {
	cluster, err := Load(clusterName)
	if err != nil {
		return nil, err
	}

	clusterDir, err := ClusterDir(clusterName)
	if err != nil {
		return nil, err
	}

	return &ClusterInfo{
		Name:              clusterName,
		Status:            cluster.Status.Phase,
		KubernetesVersion: cluster.Spec.KubernetesVersion,
		Nodes:             cluster.GetTotalDesiredNodes(),
		RunningNodes:      cluster.GetTotalRunningNodes(),
		CNI:               cluster.Spec.Networking.CNI,
		CSI:               cluster.Spec.Storage.CSI,
		LoadBalancer:      cluster.Spec.Networking.LoadBalancer,
		FilePath:          filepath.Join(clusterDir, "cluster.yaml"),
		Uptime:            calculateUptime(cluster),
	}, nil
}

// AutoSwitchAfterDeletion switches to the next available cluster after deleting the current one
func AutoSwitchAfterDeletion(deletedClusterName string) error {
	// Get current cluster
	currentCluster, err := GetCurrentCluster()
	if err != nil {
		return fmt.Errorf("failed to get current cluster: %w", err)
	}

	// If the deleted cluster is not the current one, no need to switch
	if currentCluster != deletedClusterName {
		return nil
	}

	// Get all remaining clusters
	clusters, err := ListAllClusters()
	if err != nil {
		return fmt.Errorf("failed to list clusters: %w", err)
	}

	// Filter out the deleted cluster
	var remainingClusters []string
	for _, cluster := range clusters {
		if cluster != deletedClusterName {
			remainingClusters = append(remainingClusters, cluster)
		}
	}

	// If no clusters remain, clear the current cluster setting
	if len(remainingClusters) == 0 {
		ohmykubeDir, err := OhMyKubeDir()
		if err != nil {
			return fmt.Errorf("failed to get ohmykube directory: %w", err)
		}
		
		currentClusterFile := filepath.Join(ohmykubeDir, "current-cluster")
		if err := os.Remove(currentClusterFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove current cluster file: %w", err)
		}
		
		fmt.Printf("No clusters remaining. Current cluster setting cleared.\n")
		return nil
	}

	// Switch to the first available cluster
	nextCluster := remainingClusters[0]
	if err := SetCurrentCluster(nextCluster); err != nil {
		return fmt.Errorf("failed to switch to cluster '%s': %w", nextCluster, err)
	}

	fmt.Printf("Automatically switched to cluster '%s'\n", nextCluster)
	return nil
}
