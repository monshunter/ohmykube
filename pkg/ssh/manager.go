package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/monshunter/ohmykube/pkg/config"
	"github.com/monshunter/ohmykube/pkg/log"
	"golang.org/x/crypto/ssh"
)

type SSHConfig struct {
	Password      string
	SSHKeyFile    string
	SSHPubKeyFile string
	sshKey        string
	sshPubKey     string
}

// NewSSHConfig creates a new SSH configuration with auto-generated keys for the cluster
func NewSSHConfig(password string, clusterName string) (*SSHConfig, error) {
	// Ensure SSH keys exist for the cluster
	privateKeyPath, publicKeyPath, err := ensureSSHKeys(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure SSH keys: %w", err)
	}

	sshConfig := &SSHConfig{
		Password:      password,
		SSHKeyFile:    privateKeyPath,
		SSHPubKeyFile: publicKeyPath,
	}
	err = sshConfig.init()
	if err != nil {
		return nil, err
	}
	return sshConfig, nil
}

// NewSSHConfigWithKeys creates SSH configuration with provided key file paths (for backward compatibility)
func NewSSHConfigWithKeys(password string, sshKeyFile string, sshPubKeyFile string) (*SSHConfig, error) {
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

// generateSSHKeyPair generates a new RSA 4096-bit SSH key pair
func generateSSHKeyPair() (privateKey, publicKey string, err error) {
	// Generate RSA private key
	privKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Encode private key to PEM format
	privKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	}
	privateKeyBytes := pem.EncodeToMemory(privKeyPEM)

	// Generate public key in SSH format
	pubKey, err := ssh.NewPublicKey(&privKey.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate SSH public key: %w", err)
	}
	publicKeyBytes := ssh.MarshalAuthorizedKey(pubKey)

	return string(privateKeyBytes), string(publicKeyBytes), nil
}

// getSSHKeyPath returns the SSH key directory path for a cluster
func getSSHKeyPath(clusterName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	sshDir := filepath.Join(homeDir, ".ohmykube", clusterName, ".ssh")
	return sshDir, nil
}

// ensureSSHKeys ensures SSH keys exist for the cluster, generating them if necessary
func ensureSSHKeys(clusterName string) (privateKeyPath, publicKeyPath string, err error) {
	sshDir, err := getSSHKeyPath(clusterName)
	if err != nil {
		return "", "", err
	}

	privateKeyPath = filepath.Join(sshDir, "id_rsa")
	publicKeyPath = filepath.Join(sshDir, "id_rsa.pub")

	// Check if keys already exist
	if _, err := os.Stat(privateKeyPath); err == nil {
		if _, err := os.Stat(publicKeyPath); err == nil {
			log.Debugf("SSH keys already exist for cluster %s", clusterName)
			return privateKeyPath, publicKeyPath, nil
		}
	}

	// Create SSH directory if it doesn't exist
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", "", fmt.Errorf("failed to create SSH directory: %w", err)
	}

	// Generate new key pair
	log.Debugf("Generating SSH keys for cluster %s", clusterName)
	privateKey, publicKey, err := generateSSHKeyPair()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate SSH key pair: %w", err)
	}

	// Write private key
	if err := os.WriteFile(privateKeyPath, []byte(privateKey), 0600); err != nil {
		return "", "", fmt.Errorf("failed to write private key: %w", err)
	}

	// Write public key
	if err := os.WriteFile(publicKeyPath, []byte(publicKey), 0644); err != nil {
		return "", "", fmt.Errorf("failed to write public key: %w", err)
	}

	log.Debugf("SSH keys generated successfully for cluster %s", clusterName)
	return privateKeyPath, publicKeyPath, nil
}

// CleanupSSHKeys removes SSH keys for a cluster
func CleanupSSHKeys(clusterName string) error {
	sshDir, err := getSSHKeyPath(clusterName)
	if err != nil {
		return err
	}

	if _, err := os.Stat(sshDir); os.IsNotExist(err) {
		return nil
	}

	// Remove the entire SSH directory for the cluster
	if err := os.RemoveAll(sshDir); err != nil {
		return fmt.Errorf("failed to remove SSH directory: %w", err)
	}

	log.Debugf("SSH keys cleaned up for cluster %s", clusterName)
	return nil
}

// SSHManager manages SSH clients and their lifecycle
type SSHManager struct {
	cluster   *config.Cluster
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
func NewSSHManager(cluster *config.Cluster, sshConfig *SSHConfig) *SSHManager {
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
			log.Debugf("SSH connection check for node %s failed: %v", name, err)
			// Remove invalid client
			sm.mutex.Lock()
			delete(sm.clients, name)
			sm.mutex.Unlock()
		}
	}
}

func (sm *SSHManager) Address(nodeName string) string {
	node := sm.cluster.GetNodeByName(nodeName)
	if node == nil {
		log.Errorf("Node %s does not exist, cannot get IP address", nodeName)
		return ""
	}
	return node.IP
}

// GetClient gets an SSH client
func (sm *SSHManager) GetClient(nodeName string) (*Client, bool) {
	sm.mutex.RLock()
	client, exists := sm.clients[nodeName]
	sm.mutex.RUnlock()

	if !exists {
		ip := sm.Address(nodeName)
		if ip == "" {
			log.Errorf("Node %s does not have an IP address, cannot create SSH client", nodeName)
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
		log.Debugf("Creating new SSH client for node %s at %s", nodeName, ip)
		client = NewClient(ip, "22", "root", sm.sshConfig.Password, sm.sshConfig.GetSSHKey())
		err := client.Connect()
		if err != nil {
			log.Errorf("Failed to connect to node %s: %v", nodeName, err)
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

	return client.UploadFile(localPath, remotePath)
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
func (sm *SSHManager) Close() error {
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
