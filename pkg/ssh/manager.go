package ssh

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/monshunter/ohmykube/pkg/cluster"
	"github.com/monshunter/ohmykube/pkg/log"
)

type SSHConfig struct {
	Password      string
	SSHKeyFile    string
	SSHPubKeyFile string
	sshKey        string
	sshPubKey     string
}

func NewSSHConfig(password string, sshKeyFile string, sshPubKeyFile string) (*SSHConfig, error) {
	sshConfig := &SSHConfig{
		Password:      password,
		SSHKeyFile:    sshKeyFile,
		SSHPubKeyFile: sshPubKeyFile,
	}
	err := sshConfig.init()
	if err != nil {
		return nil, err
	}
	return sshConfig, nil
}

func (c *SSHConfig) init() error {
	sshKeyContent, err := os.ReadFile(c.SSHKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read SSH private key file: %w", err)
	}
	c.sshKey = string(sshKeyContent)
	sshPubKeyContent, err := os.ReadFile(c.SSHPubKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read SSH public key file: %w", err)
	}
	c.sshPubKey = string(sshPubKeyContent)
	return nil
}

func (c *SSHConfig) GetSSHKey() string {
	return c.sshKey
}

func (c *SSHConfig) GetSSHPubKey() string {
	return c.sshPubKey
}

// SSHManager manages SSH clients and their lifecycle
type SSHManager struct {
	cluster   *cluster.Cluster
	sshConfig *SSHConfig
	clients   map[string]*Client
	mutex     sync.RWMutex
	// Fields for health check and auto-reconnect
	healthCheckInterval time.Duration
	stopHealthCheck     chan struct{}
	healthCheckActive   bool
}

// GetSSHConfig returns the SSH configuration
func (sm *SSHManager) GetSSHConfig() *SSHConfig {
	return sm.sshConfig
}

// NewSSHManager creates a new SSH manager
func NewSSHManager(cluster *cluster.Cluster, sshConfig *SSHConfig) *SSHManager {
	manager := &SSHManager{
		cluster:             cluster,
		sshConfig:           sshConfig,
		clients:             make(map[string]*Client),
		healthCheckInterval: 30 * time.Second,
		stopHealthCheck:     make(chan struct{}),
	}

	// Start health check
	manager.StartHealthCheck()

	return manager
}

// StartHealthCheck starts the health check for SSH connections
func (sm *SSHManager) StartHealthCheck() {
	sm.mutex.Lock()
	if sm.healthCheckActive {
		sm.mutex.Unlock()
		return
	}
	sm.healthCheckActive = true
	sm.mutex.Unlock()

	go func() {
		ticker := time.NewTicker(sm.healthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				sm.checkAllConnections()
			case <-sm.stopHealthCheck:
				return
			}
		}
	}()
}

// StopHealthCheck stops the health check
func (sm *SSHManager) StopHealthCheck() {
	sm.mutex.Lock()
	if !sm.healthCheckActive {
		sm.mutex.Unlock()
		return
	}
	sm.healthCheckActive = false
	sm.mutex.Unlock()

	close(sm.stopHealthCheck)
}

// checkAllConnections checks the health status of all connections
func (sm *SSHManager) checkAllConnections() {
	sm.mutex.RLock()
	clientsCopy := make(map[string]*Client)
	maps.Copy(clientsCopy, sm.clients)
	sm.mutex.RUnlock()

	for name, client := range clientsCopy {
		// Check connection
		err := client.Connect()
		if err != nil {
			log.Infof("SSH connection check for node %s failed: %v", name, err)
			// Remove invalid client
			sm.mutex.Lock()
			delete(sm.clients, name)
			sm.mutex.Unlock()
		}
	}
}

func (sm *SSHManager) GetIP(nodeName string) string {
	node := sm.cluster.GetNodeByName(nodeName)
	if node == nil {
		return ""
	}
	return node.Status.IP
}

// GetClient gets an SSH client
func (sm *SSHManager) GetClient(nodeName string) (*Client, bool) {
	sm.mutex.RLock()
	client, exists := sm.clients[nodeName]
	sm.mutex.RUnlock()

	if !exists {
		ip := sm.GetIP(nodeName)
		if ip == "" {
			return nil, false
		}

		// Create a new client
		sm.mutex.Lock()
		// Double-check locking
		client, exists = sm.clients[nodeName]
		if exists {
			sm.mutex.Unlock()
			return client, true
		}

		client = NewClient(ip, "22", "root", sm.sshConfig.Password, sm.sshConfig.GetSSHKey())
		err := client.Connect()
		if err != nil {
			sm.mutex.Unlock()
			return nil, false
		}
		sm.clients[nodeName] = client
		sm.mutex.Unlock()
		return client, true
	}

	return client, true
}

func (sm *SSHManager) CreateClient(nodeName string, ip string) (*Client, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Check if already exists
	if client, exists := sm.clients[nodeName]; exists {
		client.Close()
		delete(sm.clients, nodeName)
	}

	client := NewClient(ip, "22", "root", sm.sshConfig.Password, sm.sshConfig.GetSSHKey())
	err := client.Connect()
	if err != nil {
		return nil, err
	}
	sm.clients[nodeName] = client
	return client, nil
}

// RunCommand executes a command on the specified node
func (sm *SSHManager) RunCommand(nodeName, command string) (string, error) {
	client, exists := sm.GetClient(nodeName)
	if !exists {
		return "", fmt.Errorf("SSH client for node %s does not exist", nodeName)
	}

	return client.RunCommand(command)
}

// RunSSHCommand implements the SSHCommandRunner interface (for backward compatibility)
func (sm *SSHManager) RunSSHCommand(nodeName, command string) (string, error) {
	return sm.RunCommand(nodeName, command)
}

// UploadFile uploads a local file to a remote node using proper SCP protocol
func (sm *SSHManager) UploadFile(nodeName, localPath, remotePath string) error {
	client, exists := sm.GetClient(nodeName)
	if !exists {
		return fmt.Errorf("SSH client for node %s does not exist", nodeName)
	}

	// Ensure remote directory exists
	remoteDir := filepath.Dir(remotePath)
	if remoteDir != "." && remoteDir != "/" {
		createDirCmd := fmt.Sprintf("sudo mkdir -p %s", remoteDir)
		if _, err := client.RunCommand(createDirCmd); err != nil {
			return fmt.Errorf("failed to create remote directory %s: %w", remoteDir, err)
		}
	}

	return client.TransferFile(localPath, remotePath)
}

// DownloadFile downloads a file from a remote node to local path using proper SCP protocol
func (sm *SSHManager) DownloadFile(nodeName, remotePath, localPath string) error {
	client, exists := sm.GetClient(nodeName)
	if !exists {
		return fmt.Errorf("SSH client for node %s does not exist", nodeName)
	}

	return client.DownloadFile(remotePath, localPath)
}

// TransferFile implements the SSHFileTransfer interface (for backward compatibility)
func (sm *SSHManager) TransferFile(nodeName, localPath, remotePath string) error {
	return sm.UploadFile(nodeName, localPath, remotePath)
}

// CloseClient closes the SSH client for the specified node
func (sm *SSHManager) CloseClient(nodeName string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if client, exists := sm.clients[nodeName]; exists {
		client.Close()
		delete(sm.clients, nodeName)
	}
}

// CloseAllClients closes all SSH clients
func (sm *SSHManager) CloseAllClients() error {
	sm.StopHealthCheck()

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for nodeName, client := range sm.clients {
		if err := client.Close(); err != nil {
			return fmt.Errorf("failed to close SSH client for node %s: %w", nodeName, err)
		}
		delete(sm.clients, nodeName)
	}
	return nil
}
