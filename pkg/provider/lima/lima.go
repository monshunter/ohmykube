package lima

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
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/provider/options"
	"github.com/monshunter/ohmykube/pkg/provider/types"
	"github.com/monshunter/ohmykube/pkg/utils"
	"gopkg.in/yaml.v3"
)

// LimaProvider implements the provider.Provider interface for Lima
type LimaProvider struct {
	// Configuration options
	option *options.Options
}

// NewLimaProvider creates a new LimaProvider
func NewLimaProvider(opt *options.Options) (*LimaProvider, error) {
	// Ensure limactl is installed
	if err := checkLimactlInstalled(); err != nil {
		return nil, err
	}

	// Check if socket_vmnet daemon is running for shared network support
	// This is required when using lima:shared network configuration
	if err := checkSocketVmnetDaemon(); err != nil {
		return nil, err
	}

	if opt.Template == "" {
		opt.Template = "template://ubuntu-24.04"
	}

	if !(strings.HasSuffix(opt.Template, ".yaml") || strings.HasSuffix(opt.Template, ".yml")) &&
		!strings.HasPrefix(opt.Template, "template://") {
		opt.Template = "template://" + opt.Template
	}

	return &LimaProvider{
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
		log.Debugf("socket_vmnet check skipped on %s (primarily needed on macOS)", runtime.GOOS)
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
func (c *LimaProvider) createLimactlCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("limactl", args...)
	// Always use OhMyKube's dedicated Lima home to avoid conflicts with global Lima
	limaHome := envar.OhMyKubeLimaHome()
	env := os.Environ()
	env = append(env, fmt.Sprintf("LIMA_HOME=%s", limaHome))
	cmd.Env = env
	return cmd
}

// Name returns the provider name
func (c *LimaProvider) Name() string {
	return "lima"
}

func (c *LimaProvider) Template() string {
	return c.option.Template
}

// Info gets information about a VM
func (c *LimaProvider) Info(name string) error {
	cmd := c.createLimactlCommand("list", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to get VM information: %w, error message: %s", err, stderr.String())
	}
	return nil
}

// Create creates a new virtual machine
func (c *LimaProvider) Create(name string, args ...any) error {
	// Check if VM already exists
	status, err := c.Status(name)
	if err == nil {
		if status.IsRunning() {
			log.Debugf("VM %s already created and running, skipping creation", name)
			return nil
		} else if status.IsStopped() {
			log.Debugf("VM %s already created and stopped, starting it", name)
			err := c.Start(name)
			if err != nil {
				return err
			}
			return nil
		} else if status.IsUnknown() {
			log.Debugf("VM %s already created and unknown status, starting it", name)
			err := c.Start(name)
			if err != nil {
				return err
			}
			return nil
		} else {
			return fmt.Errorf("VM %s already created and unknown status, cannot start it", name)
		}
	}

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

	return nil
}

// checkInitAuthStatus checks if initAuth has been completed by checking the status file
func (c *LimaProvider) checkInitAuthStatus(name string) (bool, error) {
	// Check if the initAuth completion marker file exists in a persistent location
	checkCmd := `test -f /var/lib/ohmykube/initauth-complete && echo "COMPLETE" || echo "INCOMPLETE"`

	output, err := c.Exec(name, checkCmd)
	if err != nil {
		log.Debugf("Failed to check initAuth status for VM %s: %v", name, err)
		return false, nil // Return false but no error, as this might be expected during initial setup
	}

	isComplete := strings.TrimSpace(output) == "COMPLETE"
	log.Debugf("InitAuth status check for VM %s: %v", name, isComplete)
	return isComplete, nil
}

// updateInitAuthStatus creates the completion marker file to indicate initAuth is complete
func (c *LimaProvider) updateInitAuthStatus(name string) error {
	// Create completion marker file to indicate initAuth is complete
	markerCmd := `sudo mkdir -p /var/lib/ohmykube && sudo touch /var/lib/ohmykube/initauth-complete`
	if _, err := c.Exec(name, markerCmd); err != nil {
		return fmt.Errorf("failed to create initAuth completion marker: %w", err)
	}
	log.Debugf("InitAuth completion marker created for VM %s", name)
	return nil
}

// InitAuth initializes authentication for the VM
func (c *LimaProvider) InitAuth(name string) error {
	// First check if initAuth has already been completed
	isComplete, err := c.checkInitAuthStatus(name)
	if err != nil {
		log.Debugf("Failed to check initAuth status for VM %s, proceeding with initialization: %v", name, err)
	} else if isComplete {
		log.Debugf("InitAuth already completed for VM %s, skipping initialization", name)
		return nil
	}

	log.Debugf("Initializing authentication for VM %s", name)

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

	// Mark initAuth as complete
	if err := c.updateInitAuthStatus(name); err != nil {
		return fmt.Errorf("failed to update initAuth status: %w", err)
	}

	log.Debugf("InitAuth completed successfully for VM %s", name)
	return nil
}

// Delete deletes a virtual machine
func (c *LimaProvider) Delete(name string) error {
	status, err := c.Status(name)
	if err != nil {
		return err
	}

	if status.IsRunning() {
		// Stop the VM
		var stderr bytes.Buffer

		cmd := c.createLimactlCommand("stop", name)
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to stop VM: %w, error message: %s", err, stderr.String())
		}
	}

	// Delete the VM
	cmd := c.createLimactlCommand("delete", name, "--force")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete VM: %w, error message: %s", err, stderr.String())
	}

	return nil
}

// Exec executes a command on the specified virtual machine
func (c *LimaProvider) Exec(vmName, command string) (string, error) {
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
func (c *LimaProvider) List() ([]string, error) {
	if c.option.OutputFormat != "" {
		return c.listWithFormat()
	}
	return c.quietList()
}

func (c *LimaProvider) listWithFormat() ([]string, error) {
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

func (c *LimaProvider) quietList() ([]string, error) {
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
func (c *LimaProvider) GetAddress(name string) (string, error) {
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
func (c *LimaProvider) Start(name string) error {
	cmd := c.createLimactlCommand("start", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start VM %s: %w, error message: %s", name, err, stderr.String())
	}

	return nil
}

// Stop stops a virtual machine
func (c *LimaProvider) Stop(name string) error {
	status, err := c.Status(name)
	if err != nil {
		return err
	}
	if status.IsStopped() {
		log.Debugf("VM %s is already stopped", name)
		return nil
	}

	cmd := c.createLimactlCommand("stop", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop VM %s: %w, error message: %s", name, err, stderr.String())
	}

	return nil
}

// Shell opens an interactive shell to the virtual machine
func (c *LimaProvider) Shell(name string) error {
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

// Status gets the status of a virtual machine
func (c *LimaProvider) Status(name string) (types.Status, error) {
	cmd := c.createLimactlCommand("list", name, "--format", "json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return types.StatusUnknown, fmt.Errorf("failed to get status of VM %s: %w, error message: %s", name, err, stderr.String())
	}

	var vm struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &vm); err != nil {
		return types.StatusUnknown, fmt.Errorf("failed to parse JSON output: %w", err)
	}

	if vm.Name != name {
		return types.StatusUnknown, fmt.Errorf("VM with name %s not found", name)
	}

	switch vm.Status {
	case "Running":
		return types.StatusRunning, nil
	case "Stopped":
		return types.StatusStopped, nil
	default:
		return types.StatusUnknown, nil
	}
}
