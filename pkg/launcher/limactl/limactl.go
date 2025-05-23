package limactl

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// LimactlLauncher implements the launcher.Launcher interface for Lima
type LimactlLauncher struct {
	// Configuration options
	FileOrTemplate string
	Password       string
	SSHKey         string
	SSHPubKey      string
}

// NewLimactlLauncher creates a new LimactlLauncher
func NewLimactlLauncher(fileOrTemplate, password, sshKey, sshPubKey string, parallel int) (*LimactlLauncher, error) {
	// Ensure limactl is installed
	if err := checkLimactlInstalled(); err != nil {
		return nil, err
	}

	if !(strings.HasSuffix(fileOrTemplate, ".yaml") || strings.HasPrefix(fileOrTemplate, ".yml")) {
		fileOrTemplate = "template://" + fileOrTemplate
	}

	return &LimactlLauncher{
		FileOrTemplate: fileOrTemplate,
		Password:       password,
		SSHKey:         sshKey,
		SSHPubKey:      sshPubKey,
	}, nil
}

// checkLimactlInstalled checks if the limactl command exists
func checkLimactlInstalled() error {
	_, err := exec.LookPath("limactl")
	if err != nil {
		return fmt.Errorf("limactl command not found, please install limactl first: %w", err)
	}
	return nil
}

// InfoVM gets information about a VM
func (c *LimactlLauncher) InfoVM(name string) error {
	cmd := exec.Command("limactl", "list", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to get VM information: %w, error message: %s", err, stderr.String())
	}
	return nil
}

// CreateVM creates a new virtual machine
func (c *LimactlLauncher) CreateVM(name string, cpus, memory, disk int) error {
	cpusStr := strconv.Itoa(cpus)
	memoryStr := strconv.Itoa(memory)
	diskStr := strconv.Itoa(disk)
	// shared network interface
	const networks = `.networks = [{"lima": "user-v2"}, {"lima": "shared", "interface": "eth1"}]`

	// vzNAT network interface
	// networksYqStrings := `networks=[{"lima": "user-v2"}, {"vzNAT": true, "interface": "eth1"}]`
	cmd := exec.Command("limactl", "start", c.FileOrTemplate,
		"--name", name,
		"--cpus", cpusStr,
		"--memory", memoryStr,
		"--disk", diskStr,
		// "--mount-type", "virtiofs",
		// "--network", "vzNAT",
		"--set", networks,
		"--plain",
		"--tty=false",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create VM: %w, error message: %s", err, stderr.String())
	}

	return c.InitAuth(name)
}

// InitAuth initializes authentication for the VM
func (c *LimactlLauncher) InitAuth(name string) error {
	// Set host name
	hostNameCmd := fmt.Sprintf("sudo hostnamectl set-hostname %s && sudo systemctl restart systemd-hostnamed", name)
	if _, err := c.ExecCommand(name, hostNameCmd); err != nil {
		return fmt.Errorf("failed to set host name: %w", err)
	}

	// Set root password
	passwordCmd := fmt.Sprintf("echo 'root:%s' | sudo chpasswd", c.Password)
	if _, err := c.ExecCommand(name, passwordCmd); err != nil {
		return fmt.Errorf("failed to set root password: %w", err)
	}

	// Create a new SSH config file to enable password authentication
	sshConfigCmd := `sudo bash -c "echo -e 'PasswordAuthentication yes\nPermitRootLogin yes' | sudo tee /etc/ssh/sshd_config.d/99-ohmykube-settings.conf > /dev/null && sudo systemctl restart ssh || sudo systemctl restart sshd"`
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
func (c *LimactlLauncher) DeleteVM(name string) error {
	// Stop the VM
	cmd := exec.Command("limactl", "stop", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop VM: %w, error message: %s", err, stderr.String())
	}

	// Delete the VM
	cmd = exec.Command("limactl", "delete", name, "--force")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete VM: %w, error message: %s", err, stderr.String())
	}

	return nil
}

// ExecCommand executes a command on the specified virtual machine
func (c *LimactlLauncher) ExecCommand(vmName, command string) (string, error) {
	// Use limactl shell to execute commands in the VM
	cmd := exec.Command("limactl", "shell", vmName, "bash", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to execute command on VM %s: %w, error message: %s", vmName, err, stderr.String())
	}

	return stdout.String(), nil
}

// ListVM lists all virtual machines
func (c *LimactlLauncher) ListVM(prefix string) ([]string, error) {
	cmd := exec.Command("limactl", "list", "--quiet", "--log-level", "error")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w, error message: %s", err, stderr.String())
	}

	var vmNames []string
	rawOutput := stdout.String()

	if rawOutput == "" {
		return []string{}, nil
	}

	lines := strings.Split(rawOutput, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if prefix == "" || strings.HasPrefix(line, prefix) {
			vmNames = append(vmNames, strings.TrimSpace(line))
		}
	}

	return vmNames, nil
}

// TransferFile transfers a local file to the virtual machine
func (c *LimactlLauncher) TransferFile(localPath, vmName, remotePath string) error {
	// For Lima, we can use limactl copy command
	cmd := exec.Command("limactl", "copy", localPath, fmt.Sprintf("%s:%s", vmName, remotePath))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy file to VM: %w, error message: %s", err, stderr.String())
	}

	return nil
}

// GetNodeIP gets the IP address of a node
func (c *LimactlLauncher) GetNodeIP(nodeName string) (string, error) {
	// Retry a few times, as the node may need some time to fully start
	var err error
	maxRetries := 5
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		// Lima stores IP addresses differently, let's get them from the VM directly
		ipCmd := `ip -4 addr show dev eth1 | grep -oP '(?<=inet\s)\d+(\.\d+){3}'`
		ip, err := c.ExecCommand(nodeName, ipCmd)
		if err != nil {
			// Continue retrying
			break
		}
		ip = strings.TrimSpace(ip)
		if ip != "" {
			return ip, nil
		}
		// If failed, wait a while before retrying
		time.Sleep(retryDelay)
	}

	return "", fmt.Errorf("failed to get IP address for node %s after %d retries: %w", nodeName, maxRetries, err)
}
