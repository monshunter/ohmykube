package limactl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/monshunter/ohmykube/pkg/envar"
	"github.com/monshunter/ohmykube/pkg/launcher/options"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/utils"
	"gopkg.in/yaml.v3"
)

// LimactlLauncher implements the launcher.Launcher interface for Lima
type LimactlLauncher struct {
	// Configuration options
	option *options.Options
}

// NewLimactlLauncher creates a new LimactlLauncher
func NewLimactlLauncher(opt *options.Options) (*LimactlLauncher, error) {
	// Ensure limactl is installed
	if err := checkLimactlInstalled(); err != nil {
		return nil, err
	}

	// Check if socket_vmnet daemon is running for shared network support
	// This is required when using lima:shared network configuration
	if err := checkSocketVmnetDaemon(); err != nil {
		return nil, err
	}

	if !(strings.HasSuffix(opt.Template, ".yaml") || strings.HasPrefix(opt.Template, ".yml")) {
		opt.Template = "template://" + opt.Template
	}

	return &LimactlLauncher{
		option: opt,
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

// checkSocketVmnetDaemon checks if socket_vmnet daemon is running when using shared network
func checkSocketVmnetDaemon() error {
	// socket_vmnet is primarily needed on macOS for Lima shared networking
	// On other platforms, this check may not be as critical
	if runtime.GOOS != "darwin" {
		log.Infof("socket_vmnet check skipped on %s (primarily needed on macOS)", runtime.GOOS)
		return nil
	}

	isRunning, err := utils.IsProcessRunning("socket_vmnet")
	if err != nil {
		log.Warningf("Failed to check socket_vmnet daemon status: %v", err)
		return fmt.Errorf("failed to check socket_vmnet daemon: %w", err)
	}

	if !isRunning {
		log.Errorf(`Daemon check failed: socket_vmnet daemon is not running. 
			Lima shared network requires socket_vmnet daemon to be running. 
			Please install and start socket_vmnet:
			Refer to https://lima-vm.io/docs/config/network/vmnet/ for more information.`)
		return fmt.Errorf("socket_vmnet daemon is not running")
	}
	return nil
}

// createLimactlCommand creates a limactl command with the proper LIMA_HOME environment variable
func (c *LimactlLauncher) createLimactlCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("limactl", args...)
	// Always use OhMyKube's dedicated Lima home to avoid conflicts with global Lima
	limaHome := envar.OhMyKubeLimaHome()
	env := os.Environ()
	env = append(env, fmt.Sprintf("LIMA_HOME=%s", limaHome))
	cmd.Env = env
	return cmd
}

func (c *LimactlLauncher) Name() string {
	return "limactl"
}

// Info gets information about a VM
func (c *LimactlLauncher) Info(name string) error {
	cmd := c.createLimactlCommand("list", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to get VM information: %w, error message: %s", err, stderr.String())
	}
	return nil
}

// Create creates a new virtual machine
func (c *LimactlLauncher) Create(name string, args ...any) error {
	if len(args) != 3 {
		return fmt.Errorf("invalid arguments, expected 3 arguments: cpus, memory, disk")
	}
	cpus, ok := args[0].(int)
	if !ok {
		return fmt.Errorf("invalid argument type, expected int for cpus")
	}
	memory, ok := args[1].(int)
	if !ok {
		return fmt.Errorf("invalid argument type, expected int for memory")
	}
	disk, ok := args[2].(int)
	if !ok {
		return fmt.Errorf("invalid argument type, expected int for disk")
	}

	cpusStr := strconv.Itoa(cpus)
	memoryStr := strconv.Itoa(memory)
	diskStr := strconv.Itoa(disk)
	// shared network interface
	const networks = `.networks = [{"lima": "user-v2"}, {"lima": "shared", "interface": "eth1"}]`

	// vzNAT network interface
	// networksYqStrings := `networks=[{"lima": "user-v2"}, {"vzNAT": true, "interface": "eth1"}]`
	cmd := c.createLimactlCommand("start", c.option.Template,
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

	return c.initAuth(name)
}

// InitAuth initializes authentication for the VM
func (c *LimactlLauncher) initAuth(name string) error {
	// Set host name
	hostNameCmd := fmt.Sprintf("sudo hostnamectl set-hostname %s && sudo systemctl restart systemd-hostnamed", name)
	if _, err := c.Exec(name, hostNameCmd); err != nil {
		return fmt.Errorf("failed to set host name: %w", err)
	}

	// Set root password
	passwordCmd := fmt.Sprintf("echo 'root:%s' | sudo chpasswd", c.option.Password)
	if _, err := c.Exec(name, passwordCmd); err != nil {
		return fmt.Errorf("failed to set root password: %w", err)
	}

	// Create a new SSH config file to enable password authentication
	sshConfigCmd := `sudo bash -c "echo -e 'PasswordAuthentication yes\nPermitRootLogin yes' | sudo tee /etc/ssh/sshd_config.d/99-ohmykube-settings.conf > /dev/null && sudo systemctl restart ssh || sudo systemctl restart sshd"`
	if _, err := c.Exec(name, sshConfigCmd); err != nil {
		return fmt.Errorf("failed to configure SSH password authentication: %w", err)
	}

	// Create root .ssh directory and set permissions
	sshDirCmd := `sudo bash -c "mkdir -p /root/.ssh && chmod 700 /root/.ssh"`
	if _, err := c.Exec(name, sshDirCmd); err != nil {
		return fmt.Errorf("failed to create SSH directories: %w", err)
	}

	// Add SSH public key to authorized_keys for root user
	if c.option.SSHPubKey != "" {
		rootKeyCmd := fmt.Sprintf("sudo bash -c \"echo '%s' >> /root/.ssh/authorized_keys\"", c.option.SSHPubKey)
		if _, err := c.Exec(name, rootKeyCmd); err != nil {
			return fmt.Errorf("failed to add SSH public key for root user: %w", err)
		}
	}

	// Copy SSH private key for root user
	if c.option.SSHKey != "" {
		rootPrivKeyCmd := fmt.Sprintf("sudo bash -c \"echo '%s' > /root/.ssh/id_rsa && chmod 600 /root/.ssh/id_rsa\"", c.option.SSHKey)
		if _, err := c.Exec(name, rootPrivKeyCmd); err != nil {
			return fmt.Errorf("failed to configure SSH private key for root user: %w", err)
		}
	}

	return nil
}

// Delete deletes a virtual machine
func (c *LimactlLauncher) Delete(name string) error {
	// Stop the VM
	cmd := c.createLimactlCommand("stop", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop VM: %w, error message: %s", err, stderr.String())
	}

	// Delete the VM
	cmd = c.createLimactlCommand("delete", name, "--force")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete VM: %w, error message: %s", err, stderr.String())
	}

	return nil
}

// Exec executes a command on the specified virtual machine
func (c *LimactlLauncher) Exec(vmName, command string) (string, error) {
	// Use limactl shell to execute commands in the VM
	cmd := c.createLimactlCommand("shell", vmName, "bash", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to execute command on VM %s: %w, error message: %s", vmName, err, stderr.String())
	}

	return stdout.String(), nil
}

// List lists all virtual machines
func (c *LimactlLauncher) List() ([]string, error) {
	if c.option.OutputFormat != "" {
		return c.listWithFormat()
	}
	return c.quietList()
}

func (c *LimactlLauncher) listWithFormat() ([]string, error) {
	prefix := c.option.Prefix
	cmd := c.createLimactlCommand("list", "--format", c.option.OutputFormat, "--log-level", "error")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to list VMs with format: %w, error message: %s", err, stderr.String())
	}

	var virtualMachines []string
	rawOutput := stdout.String()
	// fmt.Println(rawOutput)
	if rawOutput == "" {
		return []string{}, nil
	}

	switch c.option.OutputFormat {
	case "":
		lines := strings.Split(rawOutput, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			if prefix == "" || strings.HasPrefix(line, prefix) {
				virtualMachines = append(virtualMachines, strings.TrimSpace(line))
			}
		}
	case "table":
		lines := strings.Split(rawOutput, "\n")
		if len(lines) < 1 {
			return []string{}, nil
		}
		virtualMachines = append(virtualMachines, lines[0])
		// Skip the first line, which is the header
		for _, line := range lines[1:] {
			if line == "" {
				continue
			}
			if prefix == "" || strings.HasPrefix(line, prefix) {
				virtualMachines = append(virtualMachines, strings.TrimSpace(line))
			}
		}

	case "json":
		docs := strings.Split(rawOutput, "\n")
		for _, doc := range docs {
			if doc == "" {
				continue
			}
			var vm struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal([]byte(doc), &vm); err != nil {
				return nil, fmt.Errorf("failed to parse JSON document: %w", err)
			}
			if prefix == "" || strings.HasPrefix(vm.Name, prefix) {
				virtualMachines = append(virtualMachines, doc)
			}
		}
	case "yaml":
		docs := strings.Split(rawOutput, "---")
		type instance struct {
			Name string `yaml:"name"`
		}
		type content struct {
			Instance instance `yaml:"instance"`
		}
		for _, doc := range docs {
			var vm content
			if err := yaml.Unmarshal([]byte(doc), &vm); err != nil {
				return nil, fmt.Errorf("failed to parse YAML document: %w", err)
			}
			if prefix == "" || strings.HasPrefix(vm.Instance.Name, prefix) {
				virtualMachines = append(virtualMachines, string(doc))
			}
		}
	}

	return virtualMachines, nil
}

func (c *LimactlLauncher) quietList() ([]string, error) {
	prefix := c.option.Prefix
	cmd := c.createLimactlCommand("list", "--quiet", "--log-level", "error")
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

// GetAddress gets the IP address of a node
func (c *LimactlLauncher) GetAddress(name string) (string, error) {
	// Retry a few times, as the node may need some time to fully start
	var err error
	maxRetries := 5
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		// Lima stores IP addresses differently, let's get them from the VM directly
		ipCmd := `ip -4 addr show dev eth1 | grep -oP '(?<=inet\s)\d+(\.\d+){3}'`
		ip, err := c.Exec(name, ipCmd)
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

	return "", fmt.Errorf("failed to get IP address for vm %s after %d retries: %w", name, maxRetries, err)
}

// Start starts a virtual machine
func (c *LimactlLauncher) Start(name string) error {
	cmd := c.createLimactlCommand("start", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start VM %s: %w, error message: %s", name, err, stderr.String())
	}

	return nil
}

// Stop stops a virtual machine
func (c *LimactlLauncher) Stop(name string) error {
	cmd := c.createLimactlCommand("stop", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop VM %s: %w, error message: %s", name, err, stderr.String())
	}

	return nil
}

// Shell opens an interactive shell to the virtual machine
func (c *LimactlLauncher) Shell(name string) error {
	cmd := c.createLimactlCommand("shell", name)

	// For interactive shell, we need to connect stdin/stdout/stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open shell to VM %s: %w", name, err)
	}

	return nil
}
