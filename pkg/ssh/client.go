package ssh

import (
	"fmt"
	"net"
	"os"
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

// TransferFile transfers a local file to the remote server
func (c *Client) TransferFile(localPath, remotePath string) error {
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

		// Create SCP session
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
			return lastErr
		}
		defer session.Close()

		// Create remote file
		cmd := fmt.Sprintf("cat > %s", remotePath)
		stdin, err := session.StdinPipe()
		if err != nil {
			lastErr = fmt.Errorf("failed to create standard input pipe: %w", err)
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		if err := session.Start(cmd); err != nil {
			lastErr = fmt.Errorf("failed to start SCP session: %w", err)
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		// Write file content
		if _, err := stdin.Write(data); err != nil {
			lastErr = fmt.Errorf("failed to write file data: %w", err)
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}
		stdin.Close()

		if err := session.Wait(); err != nil {
			lastErr = fmt.Errorf("failed to wait for file transfer: %w", err)
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
