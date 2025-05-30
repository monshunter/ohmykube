package clusterinfo

import (
	"fmt"
	"strings"

	"github.com/monshunter/ohmykube/pkg/interfaces"
	"github.com/monshunter/ohmykube/pkg/log"
)

// ClusterInfo provides functionality to retrieve cluster information
type ClusterInfo struct {
	sshRunner      interfaces.SSHRunner
	controllerNode string
}

// NewClusterInfo creates a new cluster information retriever
func NewClusterInfo(sshRunner interfaces.SSHRunner, controllerNode string) *ClusterInfo {
	return &ClusterInfo{
		sshRunner:      sshRunner,
		controllerNode: controllerNode,
	}
}

// GetPodCIDR retrieves the cluster Pod CIDR
func (c *ClusterInfo) GetPodCIDR() (string, error) {
	// Default value
	defaultPodCIDR := "10.244.0.0/16"

	// First try to get from kubeadm-config
	getPodCIDRFromConfigMap := `
kubectl -n kube-system get cm kubeadm-config -o jsonpath='{.data.ClusterConfiguration}' | grep -A3 networking | grep podSubnet | awk '{print $2}'
`
	output, err := c.sshRunner.RunCommand(c.controllerNode, getPodCIDRFromConfigMap)
	if err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	// If failed, try to get from kube-controller-manager startup parameters
	getFromControllerManager := `
kubectl -n kube-system get pod -l component=kube-controller-manager -o jsonpath='{.items[0].spec.containers[0].command}' | grep -o -- '--cluster-cidr=[0-9./]*' | cut -d= -f2
`
	output, err = c.sshRunner.RunCommand(c.controllerNode, getFromControllerManager)
	if err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	// Both methods failed, return default value with a warning
	log.Warningf(" Unable to get cluster Pod CIDR, using default value %s", defaultPodCIDR)
	return defaultPodCIDR, nil
}

// GetServiceCIDR retrieves the cluster Service CIDR
func (c *ClusterInfo) GetServiceCIDR() (string, error) {
	// Default value
	defaultServiceCIDR := "10.96.0.0/12"

	// First try to get from kubeadm-config
	getServiceCIDRFromConfigMap := `
kubectl -n kube-system get cm kubeadm-config -o jsonpath='{.data.ClusterConfiguration}' | grep -A5 networking | grep serviceSubnet | awk '{print $2}'
`
	output, err := c.sshRunner.RunCommand(c.controllerNode, getServiceCIDRFromConfigMap)
	if err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	// If failed, try to get from kube-apiserver startup parameters
	getFromAPIServer := `
kubectl -n kube-system get pod -l component=kube-apiserver -o jsonpath='{.items[0].spec.containers[0].command}' | grep -o -- '--service-cluster-ip-range=[0-9./]*' | cut -d= -f2
`
	output, err = c.sshRunner.RunCommand(c.controllerNode, getFromAPIServer)
	if err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	// Both methods failed, return default value with a warning
	log.Warningf(" Unable to get cluster Service CIDR, using default value %s", defaultServiceCIDR)
	return defaultServiceCIDR, nil
}

// GetKubernetesVersion retrieves the cluster Kubernetes version
func (c *ClusterInfo) GetKubernetesVersion() (string, error) {
	cmd := "kubectl version -o json | jq -r '.serverVersion.gitVersion'"
	output, err := c.sshRunner.RunCommand(c.controllerNode, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get Kubernetes version: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// GetClusterDNSDomain retrieves the cluster DNS domain
func (c *ClusterInfo) GetClusterDNSDomain() (string, error) {
	// Default value
	defaultDNSDomain := "cluster.local"

	// Try to get from kubeadm-config
	getDNSDomainFromConfigMap := `
kubectl -n kube-system get cm kubeadm-config -o jsonpath='{.data.ClusterConfiguration}' | grep -A5 networking | grep dnsDomain | awk '{print $2}'
`
	output, err := c.sshRunner.RunCommand(c.controllerNode, getDNSDomainFromConfigMap)
	if err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	// If failed, return default value
	log.Warningf(" Unable to get cluster DNS domain, using default value %s", defaultDNSDomain)
	return defaultDNSDomain, nil
}

// GetNodeCount retrieves the number of nodes in the cluster
func (c *ClusterInfo) GetNodeCount() (int, error) {
	cmd := "kubectl get nodes --no-headers | wc -l"
	output, err := c.sshRunner.RunCommand(c.controllerNode, cmd)
	if err != nil {
		return 0, fmt.Errorf("failed to get cluster node count: %w", err)
	}

	output = strings.TrimSpace(output)
	// Try to convert the output to an integer
	var count int
	_, err = fmt.Sscanf(output, "%d", &count)
	if err != nil {
		return 0, fmt.Errorf("unable to parse node count: %w", err)
	}

	return count, nil
}
