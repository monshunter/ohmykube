package ssh

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/monshunter/ohmykube/pkg/log"
	"golang.org/x/crypto/ssh"
)

// Client is a wrapper for the SSH client
type Client struct {
	Host      string
	Port      string
	User      string
	Password  string
	PrivKey   string
	client    *ssh.Client
	mu        sync.Mutex
	connected bool
}

// NewClient creates a new SSH client
func NewClient(host, port, user, password, privKey string) *Client {
	return &Client{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		PrivKey:  privKey,
	}
}

// Connect connects to the SSH server
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If already connected, return
	if c.connected && c.client != nil {
		// Check if the connection is still valid
		if err := c.testConnection(); err == nil {
			return nil
		}
		// Connection is invalid, close and reconnect
		c.client.Close()
		c.client = nil
		c.connected = false
	}

	var auth []ssh.AuthMethod

	// If password is provided, use password authentication
	if c.Password != "" {
		auth = append(auth, ssh.Password(c.Password))
	}

	// If private key is provided, use private key authentication
	if c.PrivKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(c.PrivKey))
		if err != nil {
			return fmt.Errorf("解析SSH私钥失败: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}

	config := &ssh.ClientConfig{
		User:            c.User,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second, // Increase timeout
	}

	addr := net.JoinHostPort(c.Host, c.Port)
	var err error
	var client *ssh.Client
	maxRetries := 5
	for i := range maxRetries {
		client, err = ssh.Dial("tcp", addr, config)
		if err != nil {
			if i < maxRetries-1 {
				log.Infof("ssh connection %s failed: %v, retrying (%d/%d)...", addr, err, i+1, maxRetries)
				time.Sleep(3 * time.Second)
				continue
			}
			return fmt.Errorf("ssh connection failed: %w", err)
		}
		c.client = client
		c.connected = true
		log.Infof("ssh connection %s successful", addr)
		return nil
	}
	return fmt.Errorf("ssh connection failed: %w", err)
}

// testConnection tests if the connection is valid
func (c *Client) testConnection() error {
	if c.client == nil {
		return fmt.Errorf("SSH client is nil")
	}

	// Create a temporary session to test the connection
	session, err := c.client.NewSession()
	if err != nil {
		return err
	}
	session.Close()
	return nil
}

// Close closes the SSH connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		c.connected = false
		return c.client.Close()
	}
	return nil
}

// RunCommand runs a command on the SSH server, with retry mechanism
func (c *Client) RunCommand(command string) (string, error) {
	maxRetries := 3
	var lastErr error
	var output string

	for i := 0; i < maxRetries; i++ {
		// Ensure connection
		if err := c.Connect(); err != nil {
			lastErr = err
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return "", lastErr
		}

		// Create session
		session, err := c.client.NewSession()
		if err != nil {
			lastErr = fmt.Errorf("failed to create SSH session: %w", err)
			// Connection may be disconnected, force reconnect
			c.mu.Lock()
			if c.client != nil {
				c.client.Close()
				c.client = nil
			}
			c.connected = false
			c.mu.Unlock()

			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return "", lastErr
		}

		// Ensure session is closed
		defer session.Close()

		// Execute command
		out, err := session.CombinedOutput(command)
		if err != nil {
			lastErr = fmt.Errorf("failed to execute command: %w, output: %s", err, string(out))
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return "", lastErr
		}

		output = string(out)
		return output, nil
	}

	return "", lastErr
}

// UploadFile uploads a local file to the remote server using SCP protocol
func (c *Client) UploadFile(localPath, remotePath string) error {
	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		// Ensure connection
		if err := c.Connect(); err != nil {
			lastErr = err
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		// Read local file
		data, err := os.ReadFile(localPath)
		if err != nil {
			return fmt.Errorf("failed to read local file: %w", err)
		}

		// Get file info
		fileInfo, err := os.Stat(localPath)
		if err != nil {
			return fmt.Errorf("failed to get file info: %w", err)
		}

		// Use SCP protocol for file transfer
		if err := c.scpUpload(remotePath, data, fileInfo.Mode()); err != nil {
			lastErr = fmt.Errorf("failed to upload file via SCP: %w", err)
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		return nil
	}

	return lastErr
}

// scpUpload implements SCP protocol for uploading files
func (c *Client) scpUpload(remotePath string, data []byte, mode os.FileMode) error {
	// Create SCP session
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	// Create pipes
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Start SCP command with sudo support
	scpCmd := fmt.Sprintf("sudo scp -t %s", remotePath)
	if err := session.Start(scpCmd); err != nil {
		return fmt.Errorf("failed to start SCP session: %w", err)
	}

	// Wait for initial response
	response := make([]byte, 1)
	if _, err := stdout.Read(response); err != nil {
		return fmt.Errorf("failed to read SCP response: %w", err)
	}
	if response[0] != 0 {
		return fmt.Errorf("SCP server returned error: %d", response[0])
	}

	// Send file header (mode, size, filename)
	filename := filepath.Base(remotePath)
	header := fmt.Sprintf("C%04o %d %s\n", mode&0777, len(data), filename)
	if _, err := stdin.Write([]byte(header)); err != nil {
		return fmt.Errorf("failed to send SCP header: %w", err)
	}

	// Wait for response
	if _, err := stdout.Read(response); err != nil {
		return fmt.Errorf("failed to read SCP header response: %w", err)
	}
	if response[0] != 0 {
		return fmt.Errorf("SCP server rejected header: %d", response[0])
	}

	// Send file data
	if _, err := stdin.Write(data); err != nil {
		return fmt.Errorf("failed to send file data: %w", err)
	}

	// Send end-of-file marker
	if _, err := stdin.Write([]byte{0}); err != nil {
		return fmt.Errorf("failed to send EOF marker: %w", err)
	}

	// Wait for final response
	if _, err := stdout.Read(response); err != nil {
		return fmt.Errorf("failed to read final SCP response: %w", err)
	}
	if response[0] != 0 {
		return fmt.Errorf("SCP transfer failed: %d", response[0])
	}

	stdin.Close()
	return session.Wait()
}

// DownloadFile downloads a file from the remote server using SCP protocol
func (c *Client) DownloadFile(remotePath, localPath string) error {
	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		// Ensure connection
		if err := c.Connect(); err != nil {
			lastErr = err
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		// Use SCP protocol for file download
		if err := c.scpDownload(remotePath, localPath); err != nil {
			lastErr = fmt.Errorf("failed to download file via SCP: %w", err)
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		return nil
	}

	return lastErr
}

// scpDownload implements SCP protocol for downloading files
func (c *Client) scpDownload(remotePath, localPath string) error {
	// Create SCP session
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	// Create pipes
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Start SCP command for download
	scpCmd := fmt.Sprintf("scp -f %s", remotePath)
	if err := session.Start(scpCmd); err != nil {
		return fmt.Errorf("failed to start SCP session: %w", err)
	}

	// Send ready signal
	if _, err := stdin.Write([]byte{0}); err != nil {
		return fmt.Errorf("failed to send ready signal: %w", err)
	}

	// Read file header
	headerBuf := make([]byte, 1024)
	n, err := stdout.Read(headerBuf)
	if err != nil {
		return fmt.Errorf("failed to read SCP header: %w", err)
	}

	header := string(headerBuf[:n])
	if !strings.HasPrefix(header, "C") {
		return fmt.Errorf("invalid SCP header: %s", header)
	}

	// Parse header to get file size
	parts := strings.Fields(header)
	if len(parts) < 3 {
		return fmt.Errorf("malformed SCP header: %s", header)
	}

	var fileSize int64
	if _, err := fmt.Sscanf(parts[1], "%d", &fileSize); err != nil {
		return fmt.Errorf("failed to parse file size: %w", err)
	}

	// Send acknowledgment
	if _, err := stdin.Write([]byte{0}); err != nil {
		return fmt.Errorf("failed to send acknowledgment: %w", err)
	}

	// Read file data
	data := make([]byte, fileSize)
	if _, err := stdout.Read(data); err != nil {
		return fmt.Errorf("failed to read file data: %w", err)
	}

	// Ensure local directory exists
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %w", err)
	}

	// Write to local file
	if err := os.WriteFile(localPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write local file: %w", err)
	}

	// Send final acknowledgment
	if _, err := stdin.Write([]byte{0}); err != nil {
		return fmt.Errorf("failed to send final acknowledgment: %w", err)
	}

	stdin.Close()
	return session.Wait()
}
