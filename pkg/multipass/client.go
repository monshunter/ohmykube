package multipass

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Client is a wrapper for the Multipass command-line tool
type Client struct {
	// Configuration options
	Image     string
	Password  string
	SSHKey    string
	SSHPubKey string
}

// NewClient creates a new Multipass client
func NewClient(image, password, sshKey, sshPubKey string) (*Client, error) {
	// Ensure multipass is installed
	if err := checkMultipassInstalled(); err != nil {
		return nil, err
	}
	return &Client{
		Image:     image,
		Password:  password,
		SSHKey:    sshKey,
		SSHPubKey: sshPubKey,
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
func (c *Client) InfoVM(name string) error {
	cmd := exec.Command("multipass", "info", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to get VM information: %w, error message: %s", err, stderr.String())
	}
	return nil
}

// CreateVM creates a new virtual machine
func (c *Client) CreateVM(name, cpus, memory, disk string) error {
	cmd := exec.Command("multipass", "launch", "--name", name, "--cpus", cpus, "--memory", memory, "--disk", disk, c.Image)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create VM: %w, error message: %s", err, stderr.String())
	}
	return c.InitAuth(name)
}

// InitAuth initializes authentication for the VM
func (c *Client) InitAuth(name string) error {
	// Set ubuntu and root user passwords, and enable SSH password authentication
	// Original command: multipass exec "$VM_NAME" -- bash -c "echo 'ubuntu:$NEW_PASSWORD' | sudo chpasswd && echo 'root:$NEW_PASSWORD' | sudo chpasswd && echo 'PasswordAuthentication yes' | sudo tee /etc/ssh/sshd_config.d/60-cloudimg-settings.conf > /dev/null && sudo systemctl restart ssh || sudo systemctl restart sshd"
	passwordCmd := fmt.Sprintf("echo 'ubuntu:%s' | sudo chpasswd && echo 'root:%s' | sudo chpasswd && echo 'PasswordAuthentication yes' | sudo tee /etc/ssh/sshd_config.d/60-cloudimg-settings.conf > /dev/null && sudo systemctl restart ssh || sudo systemctl restart sshd", c.Password, c.Password)
	if _, err := c.ExecCommand(name, passwordCmd); err != nil {
		return fmt.Errorf("failed to set password and SSH configuration: %w", err)
	}

	// Create .ssh directories and set permissions
	// Original command: multipass exec "$VM_NAME" -- sudo bash -c "mkdir -p /home/ubuntu/.ssh /root/.ssh && chmod 700 /home/ubuntu/.ssh /root/.ssh && chown ubuntu:ubuntu /home/ubuntu/.ssh"
	sshDirCmd := `sudo bash -c "mkdir -p /home/ubuntu/.ssh /root/.ssh && chmod 700 /home/ubuntu/.ssh /root/.ssh && chown ubuntu:ubuntu /home/ubuntu/.ssh"`
	if _, err := c.ExecCommand(name, sshDirCmd); err != nil {
		return fmt.Errorf("failed to create SSH directories: %w", err)
	}

	// Add SSH public key to authorized_keys
	if c.SSHPubKey != "" {
		// Original command: multipass exec "$VM_NAME" -- bash -c "echo '$SSH_PUB_KEY' >> /home/ubuntu/.ssh/authorized_keys"
		ubuntuKeyCmd := fmt.Sprintf("echo '%s' >> /home/ubuntu/.ssh/authorized_keys", c.SSHPubKey)
		if _, err := c.ExecCommand(name, ubuntuKeyCmd); err != nil {
			return fmt.Errorf("failed to add SSH public key for ubuntu user: %w", err)
		}

		// Original command: multipass exec "$VM_NAME" -- sudo bash -c "echo '$SSH_PUB_KEY' >> /root/.ssh/authorized_keys"
		rootKeyCmd := fmt.Sprintf("sudo bash -c \"echo '%s' >> /root/.ssh/authorized_keys\"", c.SSHPubKey)
		if _, err := c.ExecCommand(name, rootKeyCmd); err != nil {
			return fmt.Errorf("failed to add SSH public key for root user: %w", err)
		}
	}

	// Copy SSH private key
	if c.SSHKey != "" {
		// Original command: multipass exec "$VM_NAME" -- bash -c "echo '$SSH_KEY' > /home/ubuntu/.ssh/id_rsa && chmod 600 /home/ubuntu/.ssh/id_rsa && chown ubuntu:ubuntu /home/ubuntu/.ssh/id_rsa"
		ubuntuPrivKeyCmd := fmt.Sprintf("echo '%s' > /home/ubuntu/.ssh/id_rsa && chmod 600 /home/ubuntu/.ssh/id_rsa && chown ubuntu:ubuntu /home/ubuntu/.ssh/id_rsa", c.SSHKey)
		if _, err := c.ExecCommand(name, ubuntuPrivKeyCmd); err != nil {
			return fmt.Errorf("failed to configure SSH private key for ubuntu user: %w", err)
		}

		// Original command: multipass exec "$VM_NAME" -- sudo bash -c "echo '$SSH_KEY' > /root/.ssh/id_rsa && chmod 600 /root/.ssh/id_rsa"
		rootPrivKeyCmd := fmt.Sprintf("sudo bash -c \"echo '%s' > /root/.ssh/id_rsa && chmod 600 /root/.ssh/id_rsa\"", c.SSHKey)
		if _, err := c.ExecCommand(name, rootPrivKeyCmd); err != nil {
			return fmt.Errorf("failed to configure SSH private key for root user: %w", err)
		}
	}

	return nil
}

// DeleteVM deletes a virtual machine
func (c *Client) DeleteVM(name string) error {
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
func (c *Client) ExecCommand(vmName, command string) (string, error) {
	cmd := exec.Command("multipass", "exec", vmName, "--", "bash", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to execute command on VM %s: %w, error message: %s", vmName, err, stderr.String())
	}

	return stdout.String(), nil
}

// ListVMs lists all virtual machines
func (c *Client) ListVMs(prefix string) ([]string, error) {
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
func (c *Client) TransferFile(localPath, vmName, remotePath string) error {
	cmd := exec.Command("multipass", "transfer", localPath, fmt.Sprintf("%s:%s", vmName, remotePath))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to transfer file to VM: %w, error message: %s", err, stderr.String())
	}

	return nil
}
