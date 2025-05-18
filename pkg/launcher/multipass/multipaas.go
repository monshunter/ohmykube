package multipass

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// MultipassLauncher is a wrapper for the Multipass command-line tool
type MultipassLauncher struct {
	// Configuration options
	Image     string
	Password  string
	SSHKey    string
	SSHPubKey string
	semaphore chan struct{}
}

// NewMultipassLauncher creates a new Multipass MultipassLauncher
func NewMultipassLauncher(image, password, sshKey, sshPubKey string, parallel int) (*MultipassLauncher, error) {
	// Ensure multipass is installed
	if err := checkMultipassInstalled(); err != nil {
		return nil, err
	}
	return &MultipassLauncher{
		Image:     image,
		Password:  password,
		SSHKey:    sshKey,
		SSHPubKey: sshPubKey,
		semaphore: make(chan struct{}, parallel),
	}, nil
}

// checkMultipassInstalled checks if the multipass command exists
func checkMultipassInstalled() error {
	_, err := exec.LookPath("multipass")
	if err != nil {
		return fmt.Errorf("multipass command not found, please install Multipass first: %w", err)
	}
	return nil
}

// InfoVM gets information about a VM
func (c *MultipassLauncher) InfoVM(name string) error {
	cmd := exec.Command("multipass", "info", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to get VM information: %w, error message: %s", err, stderr.String())
	}
	return nil
}

// CreateVM creates a new virtual machine
func (c *MultipassLauncher) CreateVM(name string, cpus, memory, disk int) error {
	// Use semaphore to limit the number of concurrent VM creations
	c.semaphore <- struct{}{}
	defer func() { <-c.semaphore }()
	cpusStr := strconv.Itoa(cpus)
	memoryStr := strconv.Itoa(memory) + "G"
	diskStr := strconv.Itoa(disk) + "G"

	cmd := exec.Command("multipass", "launch", "--name", name, "--cpus", cpusStr, "--memory", memoryStr, "--disk", diskStr, c.Image)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create VM: %w, error message: %s", err, stderr.String())
	}
	return c.InitAuth(name)
}

// InitAuth initializes authentication for the VM
func (c *MultipassLauncher) InitAuth(name string) error {
	// Set host name
	hostNameCmd := fmt.Sprintf("sudo hostnamectl set-hostname %s && sudo systemctl restart systemd-hostnamed", name)
	if _, err := c.ExecCommand(name, hostNameCmd); err != nil {
		return fmt.Errorf("failed to set host name: %w", err)
	}
	// Set root user password
	passwordCmd := fmt.Sprintf("echo 'root:%s' | sudo chpasswd", c.Password)
	if _, err := c.ExecCommand(name, passwordCmd); err != nil {
		return fmt.Errorf("failed to set root password: %w", err)
	}

	// Create a new SSH config file to enable password authentication
	sshConfigCmd := `sudo bash -c "echo 'PasswordAuthentication yes' | sudo tee /etc/ssh/sshd_config.d/99-ohmykube-settings.conf > /dev/null && sudo systemctl restart ssh || sudo systemctl restart sshd"`
	if _, err := c.ExecCommand(name, sshConfigCmd); err != nil {
		return fmt.Errorf("failed to configure SSH password authentication: %w", err)
	}

	// Create root .ssh directory and set permissions
	sshDirCmd := `sudo bash -c "mkdir -p /root/.ssh && chmod 700 /root/.ssh"`
	if _, err := c.ExecCommand(name, sshDirCmd); err != nil {
		return fmt.Errorf("failed to create SSH directories: %w", err)
	}

	// Add SSH public key to authorized_keys for root user
	if c.SSHPubKey != "" {
		rootKeyCmd := fmt.Sprintf("sudo bash -c \"echo '%s' >> /root/.ssh/authorized_keys\"", c.SSHPubKey)
		if _, err := c.ExecCommand(name, rootKeyCmd); err != nil {
			return fmt.Errorf("failed to add SSH public key for root user: %w", err)
		}
	}

	// Copy SSH private key for root user
	if c.SSHKey != "" {
		rootPrivKeyCmd := fmt.Sprintf("sudo bash -c \"echo '%s' > /root/.ssh/id_rsa && chmod 600 /root/.ssh/id_rsa\"", c.SSHKey)
		if _, err := c.ExecCommand(name, rootPrivKeyCmd); err != nil {
			return fmt.Errorf("failed to configure SSH private key for root user: %w", err)
		}
	}

	return nil
}

// DeleteVM deletes a virtual machine
func (c *MultipassLauncher) DeleteVM(name string) error {
	// Use semaphore to limit the number of concurrent VM deletions
	c.semaphore <- struct{}{}
	defer func() { <-c.semaphore }()

	// Stop the VM
	cmd := exec.Command("multipass", "stop", name, "--force")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop VM: %w, error message: %s", err, stderr.String())
	}

	cmd = exec.Command("multipass", "delete", name)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete VM: %w, error message: %s", err, stderr.String())
	}

	// Clean up VM resources
	cmd = exec.Command("multipass", "purge")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to purge VM resources: %w", err)
	}

	return nil
}

// ExecCommand executes a command on the specified virtual machine
func (c *MultipassLauncher) ExecCommand(vmName, command string) (string, error) {
	cmd := exec.Command("multipass", "exec", vmName, "--", "bash", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to execute command on VM %s: %w, error message: %s", vmName, err, stderr.String())
	}

	return stdout.String(), nil
}

// ListVM lists all virtual machines
func (c *MultipassLauncher) ListVM(prefix string) ([]string, error) {
	cmd := exec.Command("multipass", "list", "--format", "csv")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w, error message: %s", err, stderr.String())
	}

	lines := strings.Split(stdout.String(), "\n")
	var vms []string

	// Skip header line
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) > 0 {
			vms = append(vms, parts[0])
		}
	}

	return vms, nil
}

// TransferFile transfers a local file to the virtual machine
func (c *MultipassLauncher) TransferFile(localPath, vmName, remotePath string) error {
	cmd := exec.Command("multipass", "transfer", localPath, fmt.Sprintf("%s:%s", vmName, remotePath))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to transfer file to VM: %w, error message: %s", err, stderr.String())
	}

	return nil
}

// GetNodeIP gets the IP address of a node
func (c *MultipassLauncher) GetNodeIP(nodeName string) (string, error) {
	// Retry a few times, as the node may need some time to fully start
	var err error
	maxRetries := 5
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		// Use multipass info command to get node information
		cmd := exec.Command("multipass", "info", nodeName, "--format", "json")
		output, err := cmd.Output()
		if err == nil {
			// Parse JSON output
			var info struct {
				Info map[string]struct {
					Ipv4 []string `json:"ipv4"`
				} `json:"info"`
			}

			if err := json.Unmarshal(output, &info); err != nil {
				return "", fmt.Errorf("failed to parse node information: %w", err)
			}

			nodeInfo, ok := info.Info[nodeName]
			if !ok || len(nodeInfo.Ipv4) == 0 {
				// If node info not found in this attempt, continue retrying
				time.Sleep(retryDelay)
				continue
			}

			return nodeInfo.Ipv4[0], nil
		}

		// If failed, wait a while before retrying
		time.Sleep(retryDelay)
	}

	return "", fmt.Errorf("failed to get IP address for node %s after %d retries: %w", nodeName, maxRetries, err)
}
