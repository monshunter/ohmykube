package app

import (
	"encoding/base64"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
	"github.com/spf13/cobra"
)

var registryPlainHTTP bool

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Configure containerd registry access on cluster nodes",
}

var registryConfigureCmd = &cobra.Command{
	Use:   "configure HOST:PORT",
	Short: "Configure a registry endpoint on every running cluster node",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		command, err := registryConfigureCommand(args[0], registryPlainHTTP)
		if err != nil {
			return err
		}
		return runRegistryNodeCommand(command, "configure")
	},
}

var registryRemoveCmd = &cobra.Command{
	Use:   "remove HOST:PORT",
	Short: "Remove a registry endpoint from every running cluster node",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		command, err := registryRemoveCommand(args[0])
		if err != nil {
			return err
		}
		return runRegistryNodeCommand(command, "remove")
	},
}

func init() {
	registryConfigureCmd.Flags().BoolVar(&registryPlainHTTP, "plain-http", false, "Use plain HTTP for the registry endpoint")
	registryCmd.AddCommand(registryConfigureCmd, registryRemoveCmd)
}

func normalizeRegistryEndpoint(raw string) (string, error) {
	host, portText, err := net.SplitHostPort(raw)
	if err != nil {
		return "", fmt.Errorf("registry endpoint must use HOST:PORT: %w", err)
	}
	if host == "" {
		return "", fmt.Errorf("registry endpoint host must not be empty")
	}
	if net.ParseIP(host) == nil {
		validDNS := regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?$`)
		if !validDNS.MatchString(host) {
			return "", fmt.Errorf("registry endpoint host is invalid: %q", host)
		}
		host = strings.ToLower(host)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("registry endpoint port is invalid: %q", portText)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func registryConfigureCommand(rawEndpoint string, plainHTTP bool) (string, error) {
	endpoint, err := normalizeRegistryEndpoint(rawEndpoint)
	if err != nil {
		return "", err
	}
	scheme := "https"
	if plainHTTP {
		scheme = "http"
	}
	configText := fmt.Sprintf(`server = %q

[host.%q]
  capabilities = ["pull", "resolve", "push"]
`, scheme+"://"+endpoint, scheme+"://"+endpoint)
	encoded := base64.StdEncoding.EncodeToString([]byte(configText))
	configPath := "/etc/containerd/certs.d/" + endpoint + "/hosts.toml"
	return fmt.Sprintf(
		"sudo mkdir -p /etc/containerd/certs.d/%s && printf '%%s' '%s' | base64 --decode | sudo tee %s >/dev/null && sudo systemctl restart containerd",
		endpoint,
		encoded,
		configPath,
	), nil
}

func registryRemoveCommand(rawEndpoint string) (string, error) {
	endpoint, err := normalizeRegistryEndpoint(rawEndpoint)
	if err != nil {
		return "", err
	}
	configDir := "/etc/containerd/certs.d/" + endpoint
	return fmt.Sprintf("sudo rm -rf %s && sudo systemctl restart containerd", configDir), nil
}

func runRegistryNodeCommand(command, operation string) error {
	if !config.CheckExists(clusterName) {
		return fmt.Errorf("cluster %q does not exist", clusterName)
	}
	cluster, err := config.LoadCluster(clusterName)
	if err != nil {
		return fmt.Errorf("failed to load cluster %q: %w", clusterName, err)
	}

	nodes := runningNodeNames(cluster)
	sshConfig, err := ssh.NewSSHConfig(password, clusterName)
	if err != nil {
		return fmt.Errorf("failed to create SSH configuration: %w", err)
	}
	manager := ssh.NewSSHManager(cluster, sshConfig)
	defer manager.Close()

	successCount := 0
	var lastError error
	for _, node := range nodes {
		if _, err := manager.RunCommand(node, command); err != nil {
			lastError = fmt.Errorf("node %s: %w", node, err)
			log.Errorf("Registry %s failed on node %s: %v", operation, node, err)
			continue
		}
		successCount++
	}
	if err := validateRegistryNodeResults(successCount, len(nodes), lastError); err != nil {
		return fmt.Errorf("registry %s failed: %w", operation, err)
	}
	log.Infof("Registry %s completed on all %d running nodes", operation, successCount)
	return nil
}

func runningNodeNames(cluster *config.Cluster) []string {
	var nodes []string
	for nodeName := range cluster.Nodes2IPsMap() {
		node := cluster.GetNodeByName(nodeName)
		if node != nil && node.Phase == config.PhaseRunning {
			nodes = append(nodes, nodeName)
		}
	}
	sort.Strings(nodes)
	return nodes
}

func validateRegistryNodeResults(successCount, totalNodes int, lastError error) error {
	if totalNodes == 0 {
		return fmt.Errorf("no running nodes found")
	}
	if successCount != totalNodes {
		if lastError != nil {
			return fmt.Errorf("completed on %d/%d nodes: %w", successCount, totalNodes, lastError)
		}
		return fmt.Errorf("completed on %d/%d nodes", successCount, totalNodes)
	}
	return nil
}
