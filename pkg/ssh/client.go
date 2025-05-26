package ssh

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/utils"
	"github.com/pkg/sftp"
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

// UploadFile uploads a local file to the remote server using SFTP protocol
func (c *Client) UploadFile(localPath, remotePath string) error {
	log.Infof("Starting SFTP upload: %s -> %s", localPath, remotePath)
	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			log.Infof("Retrying SFTP upload (attempt %d/%d)", i+1, maxRetries)
		}

		// Ensure connection
		if err := c.Connect(); err != nil {
			lastErr = err
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		// Use SFTP protocol for file transfer
		if err := c.sftpUpload(localPath, remotePath); err != nil {
			lastErr = fmt.Errorf("failed to upload file via SFTP: %w", err)
			if i < maxRetries-1 {
				log.Infof("SFTP upload failed, retrying: %v", err)
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		log.Infof("SFTP upload completed successfully")
		return nil
	}

	return lastErr
}

// sftpUpload implements SFTP protocol for uploading files
func (c *Client) sftpUpload(localPath, remotePath string) error {
	// Create SFTP client
	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	// Open local file
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	// Get local file info for logging and permissions
	localInfo, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get local file info: %w", err)
	}

	// Ensure remote directory exists
	remoteDir := filepath.Dir(remotePath)
	if err := sftpClient.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("failed to create remote directory: %w", err)
	}

	// Create remote file
	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %w", err)
	}
	defer remoteFile.Close()

	// Copy file using io.Copy (much faster and simpler)
	log.Infof("Uploading file: %s", utils.FormatSize(localInfo.Size()))
	_, err = io.Copy(remoteFile, localFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Set file permissions (best effort)
	if err := sftpClient.Chmod(remotePath, localInfo.Mode()); err != nil {
		log.Warningf("Failed to set remote file permissions: %v", err)
	}

	return nil
}

// DownloadFile downloads a file from the remote server using SFTP protocol
func (c *Client) DownloadFile(remotePath, localPath string) error {
	log.Infof("Starting SFTP download: %s -> %s", remotePath, localPath)
	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			log.Infof("Retrying SFTP download (attempt %d/%d)", i+1, maxRetries)
		}

		// Ensure connection
		if err := c.Connect(); err != nil {
			lastErr = err
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		// Use SFTP protocol for file download
		if err := c.sftpDownload(remotePath, localPath); err != nil {
			lastErr = fmt.Errorf("failed to download file via SFTP: %w", err)
			if i < maxRetries-1 {
				log.Infof("SFTP download failed, retrying: %v", err)
				time.Sleep(2 * time.Second)
				continue
			}
			return lastErr
		}

		log.Infof("SFTP download completed successfully")
		return nil
	}

	return lastErr
}

// sftpDownload implements SFTP protocol for downloading files
func (c *Client) sftpDownload(remotePath, localPath string) error {
	// Create SFTP client
	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	// Open remote file
	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file: %w", err)
	}
	defer remoteFile.Close()

	// Get remote file info for logging
	remoteInfo, err := remoteFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get remote file info: %w", err)
	}

	// Ensure local directory exists
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %w", err)
	}

	// Create local file
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()

	// Copy file using io.Copy (much faster and simpler)
	log.Infof("Downloading file: %s", utils.FormatSize(remoteInfo.Size()))
	_, err = io.Copy(localFile, remoteFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Set file permissions (best effort)
	if err := localFile.Chmod(remoteInfo.Mode()); err != nil {
		log.Warningf("Failed to set local file permissions: %v", err)
	}

	return nil
}
