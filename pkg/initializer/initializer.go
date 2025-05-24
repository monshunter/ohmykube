package initializer

import (
	"fmt"
	"strings"
	"time"

	"github.com/monshunter/ohmykube/pkg/default/containerd"
	"github.com/monshunter/ohmykube/pkg/default/ipvs"
	"github.com/monshunter/ohmykube/pkg/envar"
	"github.com/monshunter/ohmykube/pkg/log"
)

type osType string

const (
	osTypeDebian osType = "debian"
	osTypeRedhat osType = "redhat"
)

type Arch string

const (
	ArchAMD64 Arch = "amd64"
	ArchARM64 Arch = "arm64"
)

// Initializer used to initialize a single Kubernetes node environment
type Initializer struct {
	sshRunner SSHCommandRunner
	nodeName  string
	options   InitOptions
	osType    osType
	arch      string
	useDnf    bool
}

// NewInitializer creates a new node initializer
func NewInitializer(sshRunner SSHCommandRunner, nodeName string) (*Initializer, error) {
	initializer := &Initializer{
		sshRunner: sshRunner,
		nodeName:  nodeName,
		options:   DefaultInitOptions(),
		arch:      "arm64",
	}
	err := initializer.detectOSType()
	if err != nil {
		log.Errorf("Failed to detect OS type on node %s: %v", nodeName, err)
		return nil, err
	}
	// No need to call detectPackageManagerForRedhat separately as it's now handled in detectOSType
	return initializer, nil
}

// NewInitializerWithOptions creates a new node initializer with specified options
func NewInitializerWithOptions(sshRunner SSHCommandRunner, nodeName string, options InitOptions) (*Initializer, error) {
	initializer, err := NewInitializer(sshRunner, nodeName)
	if err != nil {
		return nil, err
	}
	initializer.options = options
	return initializer, nil
}

// DoSystemUpdate updates the system package repositories based on OS type
func (i *Initializer) DoSystemUpdate() error {
	// Call appropriate update function based on OS type
	switch i.osType {
	case osTypeDebian:
		return i.doSystemUpdateOnDebian()
	case osTypeRedhat:
		return i.doSystemUpdateOnRedhat()
	default:
		log.Infof("Unknown OS type %s on node %s, defaulting to Debian", i.osType, i.nodeName)
		return fmt.Errorf("unsupported OS type: %s", i.osType)
	}
}

// detectOSType determines if the system is Debian or RedHat based
func (i *Initializer) detectOSType() error {
	// Create a single script that performs all checks in one SSH connection
	script := `#!/bin/bash
# Check for OS type using distribution-specific files
HAS_DEBIAN_VERSION=$(test -f /etc/debian_version && echo "true" || echo "false")
HAS_REDHAT_RELEASE=$(test -f /etc/redhat-release && echo "true" || echo "false")

# Check for package managers
APT_GET_PATH=$(command -v apt-get 2>/dev/null || echo "")
YUM_PATH=$(command -v yum 2>/dev/null || echo "")
DNF_PATH=$(command -v dnf 2>/dev/null || echo "")

# Output results in a structured format
echo "DEBIAN_VERSION=${HAS_DEBIAN_VERSION}"
echo "REDHAT_RELEASE=${HAS_REDHAT_RELEASE}"
echo "APT_GET_PATH=${APT_GET_PATH}"
echo "YUM_PATH=${YUM_PATH}"
echo "DNF_PATH=${DNF_PATH}"
`

	output, err := i.sshRunner.RunSSHCommand(i.nodeName, script)
	if err != nil {
		return fmt.Errorf("failed to detect OS type on node %s: %w", i.nodeName, err)
	}

	// Parse the output
	results := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			results[parts[0]] = parts[1]
		}
	}

	// Log the results
	log.Infof("OS detection results for node %s: %v", i.nodeName, results)

	// Check for DNF availability for RedHat systems
	if results["DNF_PATH"] != "" {
		log.Infof("Detected dnf package manager on node %s", i.nodeName)
		i.useDnf = true
	}

	// Determine OS type based on the results
	if results["DEBIAN_VERSION"] == "true" {
		log.Infof("Detected Debian-based system on node %s", i.nodeName)
		i.osType = osTypeDebian
		return nil
	}

	if results["REDHAT_RELEASE"] == "true" {
		log.Infof("Detected RedHat-based system on node %s", i.nodeName)
		i.osType = osTypeRedhat
		return nil
	}

	// If we can't determine the OS type by files, check for package managers
	if results["APT_GET_PATH"] != "" {
		log.Infof("Detected apt-get on node %s, assuming Debian-based system", i.nodeName)
		i.osType = osTypeDebian
		return nil
	}

	if results["YUM_PATH"] != "" || results["DNF_PATH"] != "" {
		log.Infof("Detected yum/dnf on node %s, assuming RedHat-based system", i.nodeName)
		i.osType = osTypeRedhat
		return nil
	}

	return fmt.Errorf("failed to determine OS type on node %s", i.nodeName)
}

func (i *Initializer) doSystemUpdateOnDebian() error {
	// Update apt
	cmd := "sudo apt-get update"
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to update apt on node %s: %w", i.nodeName, err)
	}
	return nil
}

func (i *Initializer) doSystemUpdateOnRedhat() error {
	var cmd string
	if i.useDnf {
		// Use dnf if available
		log.Infof("Using dnf for system update on node %s", i.nodeName)
		cmd = "sudo dnf update -y"
	} else {
		// Fall back to yum
		log.Infof("dnf not found, using yum for system update on node %s", i.nodeName)
		cmd = "sudo yum update -y"
	}

	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to update system on node %s: %w", i.nodeName, err)
	}
	return nil
}

// AptUpdateForFixMissing runs apt-get update with fix-missing flag for Debian-based systems
func (i *Initializer) AptUpdateForFixMissing() error {
	// Call appropriate update function based on OS type
	switch i.osType {
	case osTypeDebian:
		return i.doAptUpdateForFixMissing()
	case osTypeRedhat:
		return i.doYumUpdateForFixMissing()
	default:
		log.Infof("Unknown OS type %s on node %s, defaulting to Debian", i.osType, i.nodeName)
		return fmt.Errorf("unsupported OS type: %s", i.osType)
	}
}

// doAptUpdateForFixMissing runs apt-get update with fix-missing flag
func (i *Initializer) doAptUpdateForFixMissing() error {
	// Update apt with fix-missing flag
	cmd := "sudo apt-get update --fix-missing -y"
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to update apt with fix-missing on node %s: %w", i.nodeName, err)
	}
	return nil
}

// doYumUpdateForFixMissing runs package manager clean all and update for RedHat-based systems
func (i *Initializer) doYumUpdateForFixMissing() error {
	var cmd string
	if i.useDnf {
		// Use dnf if available
		log.Infof("Using dnf for fix-missing update on node %s", i.nodeName)
		cmd = "sudo dnf clean all && sudo dnf update -y"
	} else {
		// Fall back to yum
		log.Infof("dnf not found, using yum for fix-missing update on node %s", i.nodeName)
		cmd = "sudo yum clean all && sudo yum update -y"
	}

	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to clean and update package repositories on node %s: %w", i.nodeName, err)
	}
	return nil
}

// DisableSwap disables swap
func (i *Initializer) DisableSwap() error {
	// Execute swapoff -a command
	cmd := "sudo swapoff -a"
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to disable swap on node %s: %w", i.nodeName, err)
	}

	// Modify /etc/fstab file to comment out swap lines
	cmd = "sudo sed -i '/swap/s/^/#/' /etc/fstab"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to modify /etc/fstab file on node %s: %w", i.nodeName, err)
	}

	return nil
}

// EnableIPVS enables IPVS module
func (i *Initializer) EnableIPVS() error {
	// Create a single script that performs all IPVS setup operations
	// Determine OS-specific package manager command
	var pkgInstallCmd string
	switch i.osType {
	case osTypeDebian:
		pkgInstallCmd = "sudo apt-get install -y ipvsadm ipset"
	case osTypeRedhat:
		if i.useDnf {
			pkgInstallCmd = "sudo dnf install -y ipvsadm ipset"
		} else {
			pkgInstallCmd = "sudo yum install -y ipvsadm ipset"
		}
	default:
		return fmt.Errorf("unsupported OS type: %s", i.nodeName)
	}

	// Create the script with OS-specific commands
	script := fmt.Sprintf(`#!/bin/bash
set -e

# Function to log steps and their status
log_step() {
    echo "STEP_STATUS: $1=$2"
}

# Detect OS type for logging
if [ -f /etc/debian_version ]; then
    OS_TYPE="debian"
elif [ -f /etc/redhat-release ]; then
    OS_TYPE="redhat"
    # Check for dnf
    if command -v dnf &> /dev/null; then
        PKG_MANAGER="dnf"
    else
        PKG_MANAGER="yum"
    fi
else
    OS_TYPE="unknown"
fi

echo "Detected OS: $OS_TYPE"
[ "$OS_TYPE" = "redhat" ] && echo "Package manager: $PKG_MANAGER"

# Step 1: Create modules file
echo "Creating modules file..."
cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf > /dev/null
%s
EOF
if [ $? -eq 0 ]; then
    log_step "CREATE_MODULES_FILE" "success"
else
    log_step "CREATE_MODULES_FILE" "failure"
    exit 1
fi

# Step 2: Load kernel modules
echo "Loading kernel modules..."
for module in overlay br_netfilter ip_vs ip_vs_rr ip_vs_wrr ip_vs_sh nf_conntrack; do
    echo "Loading module: $module"
    sudo modprobe $module
    if [ $? -ne 0 ]; then
        log_step "LOAD_MODULE_${module}" "failure"
        exit 1
    else
        log_step "LOAD_MODULE_${module}" "success"
    fi
done

# Step 3: Install IPVS tools
echo "Installing IPVS tools using: %s"
%s
if [ $? -eq 0 ]; then
    log_step "INSTALL_IPVS_TOOLS" "success"
else
    log_step "INSTALL_IPVS_TOOLS" "failure"
    exit 1
fi

# Step 4: Create sysctl file
echo "Creating sysctl file..."
cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf > /dev/null
%s
EOF
if [ $? -eq 0 ]; then
    log_step "CREATE_SYSCTL_FILE" "success"
else
    log_step "CREATE_SYSCTL_FILE" "failure"
    exit 1
fi

# Step 5: Apply sysctl settings
echo "Applying sysctl settings..."
sudo sysctl --system > /dev/null
if [ $? -eq 0 ]; then
    log_step "APPLY_SYSCTL" "success"
else
    log_step "APPLY_SYSCTL" "failure"
    exit 1
fi

log_step "OVERALL" "success"
exit 0
`, ipvs.K8S_MODULES_CONFIG, pkgInstallCmd, pkgInstallCmd, ipvs.K8S_SYSCTL_CONFIG)

	// Execute the script in a single SSH connection
	log.Infof("Enabling IPVS on node %s with OS type %s", i.nodeName, i.osType)
	output, err := i.sshRunner.RunSSHCommand(i.nodeName, script)
	if err != nil {
		log.Errorf("Failed to enable IPVS on node %s: %v", i.nodeName, err)
		return fmt.Errorf("failed to enable IPVS on node %s: %w", i.nodeName, err)
	}

	// Parse the output to check for any failures
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "STEP_STATUS:") {
			parts := strings.SplitN(strings.TrimPrefix(line, "STEP_STATUS: "), "=", 2)
			if len(parts) == 2 {
				step := parts[0]
				status := parts[1]
				log.Infof("IPVS setup step %s: %s on node %s", step, status, i.nodeName)

				if status == "failure" {
					return fmt.Errorf("failed to complete IPVS setup step %s on node %s", step, i.nodeName)
				}
			}
		} else if strings.HasPrefix(line, "Detected OS:") || strings.HasPrefix(line, "Package manager:") {
			// Log OS detection information
			log.Infof("%s on node %s", line, i.nodeName)
		}
	}

	log.Infof("Successfully enabled IPVS on node %s", i.nodeName)
	return nil
}

// EnableNetworkBridge enables network bridging
func (i *Initializer) EnableNetworkBridge() error {
	// Create a single script that performs all network bridge setup operations

	script := `#!/bin/bash
set -e

# Function to log steps and their status
log_step() {
    echo "STEP_STATUS: $1=$2"
}

# Step 1: Create modules file
echo "Creating modules file..."
cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf > /dev/null
overlay
br_netfilter
EOF
if [ $? -eq 0 ]; then
    log_step "CREATE_MODULES_FILE" "success"
else
    log_step "CREATE_MODULES_FILE" "failure"
    exit 1
fi

# Step 2: Load modules
echo "Loading modules..."
for module in overlay br_netfilter; do
    sudo modprobe $module
    if [ $? -ne 0 ]; then
        log_step "LOAD_MODULE_${module}" "failure"
        exit 1
    else
        log_step "LOAD_MODULE_${module}" "success"
    fi
done

# Step 3: Create sysctl file
echo "Creating sysctl file..."
cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf > /dev/null
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF
if [ $? -eq 0 ]; then
    log_step "CREATE_SYSCTL_FILE" "success"
else
    log_step "CREATE_SYSCTL_FILE" "failure"
    exit 1
fi

# Step 4: Apply sysctl settings
echo "Applying sysctl settings..."
sudo sysctl --system > /dev/null
if [ $? -eq 0 ]; then
    log_step "APPLY_SYSCTL" "success"
else
    log_step "APPLY_SYSCTL" "failure"
    exit 1
fi

log_step "OVERALL" "success"
exit 0
`

	// Execute the script in a single SSH connection
	output, err := i.sshRunner.RunSSHCommand(i.nodeName, script)
	if err != nil {
		log.Errorf("Failed to enable network bridge on node %s: %v", i.nodeName, err)
		return fmt.Errorf("failed to enable network bridge on node %s: %w", i.nodeName, err)
	}

	// Parse the output to check for any failures
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "STEP_STATUS:") {
			parts := strings.SplitN(strings.TrimPrefix(line, "STEP_STATUS: "), "=", 2)
			if len(parts) == 2 {
				step := parts[0]
				status := parts[1]
				log.Infof("Network bridge setup step %s: %s on node %s", step, status, i.nodeName)

				if status == "failure" {
					return fmt.Errorf("failed to complete network bridge setup step %s on node %s", step, i.nodeName)
				}
			}
		}
	}

	log.Infof("Successfully enabled network bridge on node %s", i.nodeName)
	return nil
}

// InstallContainerd installs and configures containerd using binary files
func (i *Initializer) InstallContainerd() error {
	// Define versions for containerd, runc, and CNI plugins
	containerdVersion := i.options.ContainerdVersion // Latest stable version as of writing
	runcVersion := i.options.RuncVersion             // Latest stable version as of writing
	cniPluginsVersion := i.options.CNIPluginsVersion // Latest stable version as of writing

	// Create a script that handles the entire containerd installation process
	script := fmt.Sprintf(`#!/bin/bash
set -e

# Function to log steps and their status
log_step() {
    echo "STEP_STATUS: $1=$2"
}

# Step 1: Check if containerd is already installed
if command -v containerd &> /dev/null; then
    CONTAINERD_VERSION=$(containerd --version 2>/dev/null | cut -d " " -f 3 | tr -d ",")
    echo "containerd already installed: $CONTAINERD_VERSION"
    log_step "CONTAINERD_CHECK" "already_installed"

    # Even if containerd is installed, we still need to configure it
    log_step "CONTINUE_CONFIG" "true"
else
    log_step "CONTAINERD_CHECK" "not_installed"
    log_step "CONTINUE_CONFIG" "true"
fi

# Step 2: Detect architecture
ARCH=%s

echo "Detected architecture: $ARCH"
log_step "ARCH_DETECTION" "success"

# Step 3: Set file paths and versions
CONTAINERD_VERSION="%s"
RUNC_VERSION="%s"
CNI_PLUGINS_VERSION="%s"

CONTAINERD_PACKAGE="containerd-${CONTAINERD_VERSION}-linux-${ARCH}.tar.gz"
CONTAINERD_PACKAGE_PATH="/usr/local/src/${CONTAINERD_PACKAGE}"
CONTAINERD_DOWNLOAD_URL="https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/${CONTAINERD_PACKAGE}"

RUNC_PACKAGE="runc.${ARCH}"
RUNC_PACKAGE_PATH="/usr/local/src/runc-${RUNC_VERSION}-${ARCH}"
RUNC_DOWNLOAD_URL="https://github.com/opencontainers/runc/releases/download/${RUNC_VERSION}/${RUNC_PACKAGE}"

CNI_PLUGINS_PACKAGE="cni-plugins-linux-${ARCH}-${CNI_PLUGINS_VERSION}.tgz"
CNI_PLUGINS_PACKAGE_PATH="/usr/local/src/${CNI_PLUGINS_PACKAGE}"
CNI_PLUGINS_DOWNLOAD_URL="https://github.com/containernetworking/plugins/releases/download/${CNI_PLUGINS_VERSION}/${CNI_PLUGINS_PACKAGE}"

# Step 4: Install dependencies
echo "Installing dependencies..."
sudo mkdir -p /usr/local/src
sudo mkdir -p /opt/cni/bin
sudo mkdir -p /etc/containerd/certs.d

# Step 5: Download and install containerd if needed
if [ -f "$CONTAINERD_PACKAGE_PATH" ]; then
    echo "containerd package already exists at $CONTAINERD_PACKAGE_PATH"
    log_step "CONTAINERD_PACKAGE_CHECK" "exists"
else
    echo "Downloading containerd from $CONTAINERD_DOWNLOAD_URL"
    sudo curl -L "$CONTAINERD_DOWNLOAD_URL" -o "$CONTAINERD_PACKAGE_PATH"
    if [ $? -eq 0 ]; then
        echo "Successfully downloaded containerd to $CONTAINERD_PACKAGE_PATH"
        log_step "DOWNLOAD_CONTAINERD" "success"
    else
        echo "Failed to download containerd"
        log_step "DOWNLOAD_CONTAINERD" "failure"
        exit 1
    fi
fi

# Extract and install containerd if not already installed
if ! command -v containerd &> /dev/null; then
    echo "Extracting and installing containerd"
    sudo tar -xzf "$CONTAINERD_PACKAGE_PATH" -C /usr/local
    if [ $? -ne 0 ]; then
        echo "Failed to extract containerd"
        log_step "EXTRACT_CONTAINERD" "failure"
        exit 1
    fi
    log_step "EXTRACT_CONTAINERD" "success"
else
    log_step "EXTRACT_CONTAINERD" "skipped"
fi

# Step 6: Download and install runc if needed
if [ -f "$RUNC_PACKAGE_PATH" ]; then
    echo "runc package already exists at $RUNC_PACKAGE_PATH"
    log_step "RUNC_PACKAGE_CHECK" "exists"
else
    echo "Downloading runc from $RUNC_DOWNLOAD_URL"
    sudo curl -L "$RUNC_DOWNLOAD_URL" -o "$RUNC_PACKAGE_PATH"
    if [ $? -eq 0 ]; then
        echo "Successfully downloaded runc to $RUNC_PACKAGE_PATH"
        log_step "DOWNLOAD_RUNC" "success"
    else
        echo "Failed to download runc"
        log_step "DOWNLOAD_RUNC" "failure"
        exit 1
    fi
fi

# Install runc
echo "Installing runc"
sudo install -m 755 "$RUNC_PACKAGE_PATH" /usr/local/bin/runc
if [ $? -ne 0 ]; then
    echo "Failed to install runc"
    log_step "INSTALL_RUNC" "failure"
    exit 1
fi
log_step "INSTALL_RUNC" "success"

# Step 7: Download and install CNI plugins if needed
if [ -f "$CNI_PLUGINS_PACKAGE_PATH" ]; then
    echo "CNI plugins package already exists at $CNI_PLUGINS_PACKAGE_PATH"
    log_step "CNI_PLUGINS_PACKAGE_CHECK" "exists"
else
    echo "Downloading CNI plugins from $CNI_PLUGINS_DOWNLOAD_URL"
    sudo curl -L "$CNI_PLUGINS_DOWNLOAD_URL" -o "$CNI_PLUGINS_PACKAGE_PATH"
    if [ $? -eq 0 ]; then
        echo "Successfully downloaded CNI plugins to $CNI_PLUGINS_PACKAGE_PATH"
        log_step "DOWNLOAD_CNI_PLUGINS" "success"
    else
        echo "Failed to download CNI plugins"
        log_step "DOWNLOAD_CNI_PLUGINS" "failure"
        exit 1
    fi
fi

# Extract and install CNI plugins
echo "Extracting and installing CNI plugins"
sudo mkdir -p /opt/cni/bin
sudo tar -xzf "$CNI_PLUGINS_PACKAGE_PATH" -C /opt/cni/bin
if [ $? -ne 0 ]; then
    echo "Failed to extract CNI plugins"
    log_step "EXTRACT_CNI_PLUGINS" "failure"
    exit 1
fi
log_step "EXTRACT_CNI_PLUGINS" "success"

# Step 8: Create containerd systemd service
echo "Creating containerd systemd service"
cat <<EOF | sudo tee /etc/systemd/system/containerd.service >/dev/null
[Unit]
Description=containerd container runtime
Documentation=https://containerd.io
After=network.target local-fs.target dbus.service

[Service]
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/containerd

Type=notify
Delegate=yes
KillMode=process
Restart=always
RestartSec=5

# Having non-zero Limit*s causes performance problems due to accounting overhead
# in the kernel. We recommend using cgroups to do container-local accounting.
LimitNPROC=infinity
LimitCORE=infinity

# Comment TasksMax if your systemd version does not supports it.
# Only systemd 226 and above support this version.
TasksMax=infinity
OOMScoreAdjust=-999

[Install]
WantedBy=multi-user.target
EOF

if [ $? -ne 0 ]; then
    echo "Failed to create containerd systemd service"
    log_step "CREATE_CONTAINERD_SERVICE" "failure"
    exit 1
fi
log_step "CREATE_CONTAINERD_SERVICE" "success"

# Step 9: Create containerd configuration directory
echo "Creating containerd configuration"
sudo mkdir -p /etc/containerd

# Step 10: Generate default containerd config
if command -v containerd &> /dev/null; then
    echo "Generating default containerd config"
    containerd config default | sudo tee /etc/containerd/config.toml.default > /dev/null
    if [ $? -ne 0 ]; then
        echo "Failed to generate default containerd config"
        log_step "GENERATE_DEFAULT_CONFIG" "failure"
    else
        log_step "GENERATE_DEFAULT_CONFIG" "success"
    fi
fi

# Step 11: Write custom configuration
echo "Writing containerd configuration"
cat <<EOF | sudo tee /etc/containerd/config.toml >/dev/null
%s
EOF

if [ $? -ne 0 ]; then
    echo "Failed to create containerd configuration file"
    log_step "CREATE_CONFIG" "failure"
    exit 1
fi
log_step "CREATE_CONFIG" "success"

# Step 12: Set up mirrors if needed
echo "Setting up containerd mirrors"
%s

# Step 13: Reload systemd and restart containerd
echo "Reloading systemd and starting containerd"
sudo systemctl daemon-reload
sudo systemctl enable containerd
sudo systemctl restart containerd
if [ $? -ne 0 ]; then
    echo "Failed to start containerd service"
    log_step "START_CONTAINERD" "failure"
    exit 1
fi
log_step "START_CONTAINERD" "success"

# Step 14: Verify installation
echo "Verifying installation"
if command -v containerd &> /dev/null && command -v runc &> /dev/null; then
    CONTAINERD_VERSION_INSTALLED=$(containerd --version 2>/dev/null | cut -d " " -f 3 | tr -d ",")
    RUNC_VERSION_INSTALLED=$(runc --version 2>/dev/null | head -1 | cut -d " " -f 3)

    echo "Successfully installed container runtime components:"
    echo "containerd: $CONTAINERD_VERSION_INSTALLED"
    echo "runc: $RUNC_VERSION_INSTALLED"
    echo "CNI plugins: v$CNI_PLUGINS_VERSION"

    log_step "VERIFY_INSTALLATION" "success"
else
    echo "Failed to verify installation of container runtime components"
    log_step "VERIFY_INSTALLATION" "failure"
    exit 1
fi

log_step "OVERALL" "success"
exit 0
`, i.arch, containerdVersion, runcVersion, cniPluginsVersion, containerd.CONFIG, getMirrorSetupScript())

	// Execute the script in a single SSH connection
	log.Infof("Installing containerd on node %s", i.nodeName)

	// Set up retry logic
	maxRetries := 3
	retryDelay := 5 * time.Second
	var output string
	var err error

	for retry := 0; retry < maxRetries; retry++ {
		output, err = i.sshRunner.RunSSHCommand(i.nodeName, script)

		// Parse the output to check for any failures or if already installed
		lines := strings.Split(strings.TrimSpace(output), "\n")
		var alreadyInstalled bool

		for _, line := range lines {
			if strings.HasPrefix(line, "STEP_STATUS:") {
				parts := strings.SplitN(strings.TrimPrefix(line, "STEP_STATUS: "), "=", 2)
				if len(parts) == 2 {
					step := parts[0]
					status := parts[1]
					log.Infof("containerd installation step %s: %s on node %s", step, status, i.nodeName)

					// Check if containerd is already installed
					if step == "CONTAINERD_CHECK" && status == "already_installed" {
						alreadyInstalled = true
						log.Infof("containerd already installed on node %s, continuing with configuration", i.nodeName)
					}

					// Check if we should continue with configuration
					if step == "CONTINUE_CONFIG" && status == "true" {
						// Configuration will continue regardless
						log.Infof("Continuing with containerd configuration on node %s", i.nodeName)
					}

					// If any step failed, log it but continue with retry logic
					if status == "failure" && retry < maxRetries-1 {
						log.Infof("containerd installation step %s failed on node %s, will retry", step, i.nodeName)
					}
				}
			} else if strings.Contains(line, "already installed") {
				log.Infof("%s on node %s", line, i.nodeName)
			}
		}

		if err == nil {
			// Check if overall process was successful
			for _, line := range lines {
				if strings.HasPrefix(line, "STEP_STATUS: OVERALL=success") {
					if alreadyInstalled {
						log.Infof("Successfully configured containerd on node %s", i.nodeName)
					} else {
						log.Infof("Successfully installed and configured containerd on node %s", i.nodeName)
					}

					// Install crictl and nerdctl after containerd is successfully installed
					if err := i.InstallCrictlAndNerdctl(); err != nil {
						log.Errorf("Failed to install crictl and nerdctl on node %s: %v", i.nodeName, err)
						// Continue even if crictl and nerdctl installation fails
					}

					return nil
				}
			}
		}

		if retry < maxRetries-1 {
			log.Infof("Failed to install containerd on node %s, retrying in %v (%d/%d)...",
				i.nodeName, retryDelay, retry+1, maxRetries)
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to install containerd on node %s after %d attempts: %w", i.nodeName, maxRetries, err)
	}

	return fmt.Errorf("failed to install containerd on node %s after %d attempts", i.nodeName, maxRetries)
}

// InstallCrictlAndNerdctl installs crictl and nerdctl tools for container management
func (i *Initializer) InstallCrictlAndNerdctl() error {
	// Define versions for crictl and nerdctl
	crictlVersion := i.options.CriCtlVersion   // Latest stable version as of writing
	nerdctlVersion := i.options.NerdctlVersion // Latest stable version as of writing

	// Create a script that handles the installation of crictl and nerdctl
	script := fmt.Sprintf(`#!/bin/bash
set -e

# Function to log steps and their status
log_step() {
    echo "STEP_STATUS: $1=$2"
}

# Step 1: Detect architecture
ARCH=%s

echo "Detected architecture: $ARCH"
log_step "ARCH_DETECTION" "success"

# Step 2: Set file paths and versions
CRICTL_VERSION="%s"
NERDCTL_VERSION="%s"

CRICTL_PACKAGE="crictl-${CRICTL_VERSION}-linux-${ARCH}.tar.gz"
CRICTL_PACKAGE_PATH="/usr/local/src/${CRICTL_PACKAGE}"
CRICTL_DOWNLOAD_URL="https://github.com/kubernetes-sigs/cri-tools/releases/download/${CRICTL_VERSION}/${CRICTL_PACKAGE}"

NERDCTL_PACKAGE="nerdctl-${NERDCTL_VERSION}-linux-${ARCH}.tar.gz"
NERDCTL_PACKAGE_PATH="/usr/local/src/${NERDCTL_PACKAGE}"
NERDCTL_DOWNLOAD_URL="https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/${NERDCTL_PACKAGE}"

# Step 3: Create directories
sudo mkdir -p /usr/local/src
sudo mkdir -p /usr/local/bin

# Step 4: Check if crictl is already installed
if command -v crictl &> /dev/null; then
    CRICTL_VERSION_INSTALLED=$(crictl --version 2>/dev/null | awk '{print $3}')
    echo "crictl already installed: $CRICTL_VERSION_INSTALLED"
    log_step "CRICTL_CHECK" "already_installed"
else
    log_step "CRICTL_CHECK" "not_installed"

    # Step 5: Download and install crictl if needed
    if [ -f "$CRICTL_PACKAGE_PATH" ]; then
        echo "crictl package already exists at $CRICTL_PACKAGE_PATH"
        log_step "CRICTL_PACKAGE_CHECK" "exists"
    else
        echo "Downloading crictl from $CRICTL_DOWNLOAD_URL"
        sudo curl -L "$CRICTL_DOWNLOAD_URL" -o "$CRICTL_PACKAGE_PATH"
        if [ $? -eq 0 ]; then
            echo "Successfully downloaded crictl to $CRICTL_PACKAGE_PATH"
            log_step "DOWNLOAD_CRICTL" "success"
        else
            echo "Failed to download crictl"
            log_step "DOWNLOAD_CRICTL" "failure"
            exit 1
        fi
    fi

    # Extract and install crictl
    echo "Extracting and installing crictl"
    sudo tar -xzf "$CRICTL_PACKAGE_PATH" -C /usr/local/bin
    if [ $? -ne 0 ]; then
        echo "Failed to extract crictl"
        log_step "EXTRACT_CRICTL" "failure"
        exit 1
    fi
    log_step "EXTRACT_CRICTL" "success"

    # Set up crictl configuration
    echo "Setting up crictl configuration"
    sudo mkdir -p /etc/crictl
    cat <<EOF | sudo tee /etc/crictl.yaml >/dev/null
runtime-endpoint: unix:///run/containerd/containerd.sock
image-endpoint: unix:///run/containerd/containerd.sock
timeout: 10
debug: false
EOF
    if [ $? -ne 0 ]; then
        echo "Failed to create crictl configuration file"
        log_step "CREATE_CRICTL_CONFIG" "failure"
        exit 1
    fi
    log_step "CREATE_CRICTL_CONFIG" "success"
fi

# Step 6: Check if nerdctl is already installed
if command -v nerdctl &> /dev/null; then
    NERDCTL_VERSION_INSTALLED=$(nerdctl --version 2>/dev/null | awk '{print $3}')
    echo "nerdctl already installed: $NERDCTL_VERSION_INSTALLED"
    log_step "NERDCTL_CHECK" "already_installed"
else
    log_step "NERDCTL_CHECK" "not_installed"

    # Step 7: Download and install nerdctl if needed
    if [ -f "$NERDCTL_PACKAGE_PATH" ]; then
        echo "nerdctl package already exists at $NERDCTL_PACKAGE_PATH"
        log_step "NERDCTL_PACKAGE_CHECK" "exists"
    else
        echo "Downloading nerdctl from $NERDCTL_DOWNLOAD_URL"
        sudo curl -L "$NERDCTL_DOWNLOAD_URL" -o "$NERDCTL_PACKAGE_PATH"
        if [ $? -eq 0 ]; then
            echo "Successfully downloaded nerdctl to $NERDCTL_PACKAGE_PATH"
            log_step "DOWNLOAD_NERDCTL" "success"
        else
            echo "Failed to download nerdctl"
            log_step "DOWNLOAD_NERDCTL" "failure"
            exit 1
        fi
    fi

    # Extract and install nerdctl
    echo "Extracting and installing nerdctl"
    sudo tar -xzf "$NERDCTL_PACKAGE_PATH" -C /usr/local/bin
    if [ $? -ne 0 ]; then
        echo "Failed to extract nerdctl"
        log_step "EXTRACT_NERDCTL" "failure"
        exit 1
    fi
    log_step "EXTRACT_NERDCTL" "success"
fi

# Step 8: Verify installation
echo "Verifying installation"
CRICTL_INSTALLED=$(command -v crictl >/dev/null 2>&1 && echo "yes" || echo "no")
NERDCTL_INSTALLED=$(command -v nerdctl >/dev/null 2>&1 && echo "yes" || echo "no")

if [ "$CRICTL_INSTALLED" = "yes" ] && [ "$NERDCTL_INSTALLED" = "yes" ]; then
    echo "Successfully installed container tools:"
    echo "crictl: $(crictl --version 2>/dev/null | awk '{print $3}')"
    echo "nerdctl: $(nerdctl --version 2>/dev/null | awk '{print $3}')"
    log_step "VERIFY_INSTALLATION" "success"
else
    echo "Failed to verify installation of container tools"
    log_step "VERIFY_INSTALLATION" "failure"
    exit 1
fi

log_step "OVERALL" "success"
exit 0
`, i.arch, crictlVersion, nerdctlVersion)

	// Execute the script in a single SSH connection
	log.Infof("Installing crictl and nerdctl on node %s", i.nodeName)

	// Set up retry logic
	maxRetries := 3
	retryDelay := 5 * time.Second
	var output string
	var err error

	for retry := 0; retry < maxRetries; retry++ {
		output, err = i.sshRunner.RunSSHCommand(i.nodeName, script)

		// Parse the output to check for any failures or if already installed
		lines := strings.Split(strings.TrimSpace(output), "\n")
		var crictlAlreadyInstalled, nerdctlAlreadyInstalled bool

		for _, line := range lines {
			if strings.HasPrefix(line, "STEP_STATUS:") {
				parts := strings.SplitN(strings.TrimPrefix(line, "STEP_STATUS: "), "=", 2)
				if len(parts) == 2 {
					step := parts[0]
					status := parts[1]
					log.Infof("Container tools installation step %s: %s on node %s", step, status, i.nodeName)

					// Check if tools are already installed
					if step == "CRICTL_CHECK" && status == "already_installed" {
						crictlAlreadyInstalled = true
						log.Infof("crictl already installed on node %s", i.nodeName)
					}
					if step == "NERDCTL_CHECK" && status == "already_installed" {
						nerdctlAlreadyInstalled = true
						log.Infof("nerdctl already installed on node %s", i.nodeName)
					}

					// If any step failed, log it but continue with retry logic
					if status == "failure" && retry < maxRetries-1 {
						log.Infof("Container tools installation step %s failed on node %s, will retry", step, i.nodeName)
					}
				}
			} else if strings.Contains(line, "already installed") {
				log.Infof("%s on node %s", line, i.nodeName)
			}
		}

		if err == nil {
			// Check if overall process was successful
			for _, line := range lines {
				if strings.HasPrefix(line, "STEP_STATUS: OVERALL=success") {
					if crictlAlreadyInstalled && nerdctlAlreadyInstalled {
						log.Infof("crictl and nerdctl already installed on node %s", i.nodeName)
					} else if crictlAlreadyInstalled {
						log.Infof("crictl already installed, successfully installed nerdctl on node %s", i.nodeName)
					} else if nerdctlAlreadyInstalled {
						log.Infof("nerdctl already installed, successfully installed crictl on node %s", i.nodeName)
					} else {
						log.Infof("Successfully installed crictl and nerdctl on node %s", i.nodeName)
					}
					return nil
				}
			}
		}

		if retry < maxRetries-1 {
			log.Infof("Failed to install container tools on node %s, retrying in %v (%d/%d)...",
				i.nodeName, retryDelay, retry+1, maxRetries)
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to install container tools on node %s after %d attempts: %w", i.nodeName, maxRetries, err)
	}

	return fmt.Errorf("failed to install container tools on node %s after %d attempts", i.nodeName, maxRetries)
}

// getMirrorSetupScript generates the script for setting up containerd mirrors
func getMirrorSetupScript() string {
	if !envar.IsEnableDefaultMiror() {
		return "# No mirrors configured"
	}

	var mirrorScript strings.Builder
	for _, mirror := range containerd.Mirrors() {
		mirrorScript.WriteString(fmt.Sprintf(`
sudo mkdir -p /etc/containerd/certs.d/%s
cat <<EOF | sudo tee /etc/containerd/certs.d/%s/hosts.toml >/dev/null
%s
EOF
`, mirror.Name, mirror.Name, mirror.Config))
	}

	return mirrorScript.String()
}

// InstallK8sComponents installs kubeadm, kubectl, kubelet using binary files
func (i *Initializer) InstallK8sComponents() error {
	// Define the Kubernetes version to install
	// Create a script that handles the entire Kubernetes components installation process
	script := fmt.Sprintf(`#!/bin/bash
set -e

# Function to log steps and their status
log_step() {
    echo "STEP_STATUS: $1=$2"
}

# Step 1: Check if components are already installed
if command -v kubelet &> /dev/null && command -v kubeadm &> /dev/null && command -v kubectl &> /dev/null; then
    KUBELET_VERSION=$(kubelet --version 2>/dev/null | cut -d " " -f 2)
    KUBEADM_VERSION=$(kubeadm version -o short 2>/dev/null)
    KUBECTL_VERSION=$(kubectl version --client -o yaml 2>/dev/null | grep -i gitVersion | head -1 | cut -d ":" -f 2 | tr -d " ")
    echo "Kubernetes components already installed:"
    echo "kubelet: $KUBELET_VERSION"
    echo "kubeadm: $KUBEADM_VERSION"
    echo "kubectl: $KUBECTL_VERSION"
    log_step "K8S_CHECK" "already_installed"
    exit 0
fi

# Step 2: Detect architecture
ARCH=%s
echo "Detected architecture: $ARCH"
log_step "ARCH_DETECTION" "success"

# Step 3: Set file paths and versions
K8S_VERSION="%s"
DOWNLOAD_BASE_URL="https://dl.k8s.io/release/$K8S_VERSION/bin/linux/$ARCH"
KUBECTL_PACKAGE="/usr/local/src/kubectl-$K8S_VERSION-$ARCH"
KUBEADM_PACKAGE="/usr/local/src/kubeadm-$K8S_VERSION-$ARCH"
KUBELET_PACKAGE="/usr/local/src/kubelet-$K8S_VERSION-$ARCH"

# Step 4: Download kubectl if needed
if [ -f "$KUBECTL_PACKAGE" ]; then
    echo "kubectl package already exists at $KUBECTL_PACKAGE"
    log_step "KUBECTL_PACKAGE_CHECK" "exists"
else
    echo "Downloading kubectl from $DOWNLOAD_BASE_URL/kubectl"
    sudo mkdir -p /usr/local/src
    sudo curl -L "$DOWNLOAD_BASE_URL/kubectl" -o "$KUBECTL_PACKAGE"
    if [ $? -eq 0 ]; then
        echo "Successfully downloaded kubectl to $KUBECTL_PACKAGE"
        log_step "DOWNLOAD_KUBECTL" "success"
    else
        echo "Failed to download kubectl"
        log_step "DOWNLOAD_KUBECTL" "failure"
        exit 1
    fi
fi

# Step 5: Download kubeadm if needed
if [ -f "$KUBEADM_PACKAGE" ]; then
    echo "kubeadm package already exists at $KUBEADM_PACKAGE"
    log_step "KUBEADM_PACKAGE_CHECK" "exists"
else
    echo "Downloading kubeadm from $DOWNLOAD_BASE_URL/kubeadm"
    sudo curl -L "$DOWNLOAD_BASE_URL/kubeadm" -o "$KUBEADM_PACKAGE"
    if [ $? -eq 0 ]; then
        echo "Successfully downloaded kubeadm to $KUBEADM_PACKAGE"
        log_step "DOWNLOAD_KUBEADM" "success"
    else
        echo "Failed to download kubeadm"
        log_step "DOWNLOAD_KUBEADM" "failure"
        exit 1
    fi
fi

# Step 6: Download kubelet if needed
if [ -f "$KUBELET_PACKAGE" ]; then
    echo "kubelet package already exists at $KUBELET_PACKAGE"
    log_step "KUBELET_PACKAGE_CHECK" "exists"
else
    echo "Downloading kubelet from $DOWNLOAD_BASE_URL/kubelet"
    sudo curl -L "$DOWNLOAD_BASE_URL/kubelet" -o "$KUBELET_PACKAGE"
    if [ $? -eq 0 ]; then
        echo "Successfully downloaded kubelet to $KUBELET_PACKAGE"
        log_step "DOWNLOAD_KUBELET" "success"
    else
        echo "Failed to download kubelet"
        log_step "DOWNLOAD_KUBELET" "failure"
        exit 1
    fi
fi

# Step 7: Install kubectl
echo "Installing kubectl"
sudo install -o root -g root -m 0755 "$KUBECTL_PACKAGE" /usr/bin/kubectl
if [ $? -ne 0 ]; then
    echo "Failed to install kubectl"
    log_step "INSTALL_KUBECTL" "failure"
    exit 1
fi

log_step "INSTALL_KUBECTL" "success"

# Step 8: Install kubeadm
echo "Installing kubeadm"
sudo install -o root -g root -m 0755 "$KUBEADM_PACKAGE" /usr/bin/kubeadm
if [ $? -ne 0 ]; then
    echo "Failed to install kubeadm"
    log_step "INSTALL_KUBEADM" "failure"
    exit 1
fi

log_step "INSTALL_KUBEADM" "success"

# Step 9: Install kubelet
echo "Installing kubelet"
sudo install -o root -g root -m 0755 "$KUBELET_PACKAGE" /usr/bin/kubelet
if [ $? -ne 0 ]; then
    echo "Failed to install kubelet"
    log_step "INSTALL_KUBELET" "failure"
    exit 1
fi

log_step "INSTALL_KUBELET" "success"

# Step 10: Create kubelet systemd service
echo "Creating kubelet systemd service"
sudo mkdir -p /etc/systemd/system/kubelet.service.d
cat <<EOF | sudo tee /etc/systemd/system/kubelet.service >/dev/null
[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=https://kubernetes.io/docs/
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=/usr/bin/kubelet
Restart=always
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

cat <<EOF | sudo tee /etc/systemd/system/kubelet.service.d/10-kubeadm.conf >/dev/null
# Note: This dropin only works with kubeadm and kubelet v1.11+
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
# This is a file that "kubeadm init" and "kubeadm join" generates at runtime, populating the KUBELET_KUBEADM_ARGS variable dynamically
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
# This is a file that the user can use for overrides of the kubelet args as a last resort. Preferably, the user should use
# the .NodeRegistration.KubeletExtraArgs object in the configuration files instead. KUBELET_EXTRA_ARGS should be sourced from this file.
EnvironmentFile=-/etc/default/kubelet
ExecStart=
ExecStart=/usr/bin/kubelet \$KUBELET_KUBECONFIG_ARGS \$KUBELET_CONFIG_ARGS \$KUBELET_KUBEADM_ARGS \$KUBELET_EXTRA_ARGS
EOF

if [ $? -ne 0 ]; then
    echo "Failed to create kubelet systemd service"
    log_step "CREATE_KUBELET_SERVICE" "failure"
    exit 1
fi
log_step "CREATE_KUBELET_SERVICE" "success"

# Step 11: Create directories needed by kubelet
echo "Creating kubelet directories"
sudo mkdir -p /etc/kubernetes/manifests
sudo mkdir -p /var/lib/kubelet
sudo mkdir -p /var/log/kubernetes
if [ $? -ne 0 ]; then
    echo "Failed to create kubelet directories"
    log_step "CREATE_KUBELET_DIRS" "failure"
    exit 1
fi
log_step "CREATE_KUBELET_DIRS" "success"

# Step 12: Enable and start kubelet service
echo "Enabling kubelet service"
sudo systemctl daemon-reload
sudo systemctl enable kubelet
if [ $? -ne 0 ]; then
    echo "Failed to enable kubelet service"
    log_step "ENABLE_KUBELET" "failure"
    exit 1
fi
log_step "ENABLE_KUBELET" "success"

# Step 13: Verify installation
echo "Verifying installation"
KUBECTL_INSTALLED=$(command -v kubectl >/dev/null 2>&1 && echo "yes" || echo "no")
KUBEADM_INSTALLED=$(command -v kubeadm >/dev/null 2>&1 && echo "yes" || echo "no")
KUBELET_INSTALLED=$(command -v kubelet >/dev/null 2>&1 && echo "yes" || echo "no")

if [ "$KUBECTL_INSTALLED" = "yes" ] && [ "$KUBEADM_INSTALLED" = "yes" ] && [ "$KUBELET_INSTALLED" = "yes" ]; then
    echo "Successfully installed Kubernetes components:"
    echo "kubectl: $(kubectl version --client -o yaml 2>/dev/null | grep -i gitVersion | head -1 | cut -d ":" -f 2 | tr -d " ")"
    echo "kubeadm: $(kubeadm version -o short 2>/dev/null)"
    echo "kubelet: $(kubelet --version 2>/dev/null | cut -d " " -f 2)"
    log_step "VERIFY_INSTALLATION" "success"
else
    echo "Failed to verify installation of Kubernetes components"
    log_step "VERIFY_INSTALLATION" "failure"
    exit 1
fi

log_step "OVERALL" "success"
exit 0
`, i.arch, i.options.K8SVersion)

	// Execute the script in a single SSH connection
	log.Infof("Installing Kubernetes components on node %s", i.nodeName)

	// Set up retry logic
	maxRetries := 3
	retryDelay := 5 * time.Second
	var output string
	var err error

	for retry := 0; retry < maxRetries; retry++ {
		output, err = i.sshRunner.RunSSHCommand(i.nodeName, script)

		// Parse the output to check for any failures or if already installed
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "STEP_STATUS:") {
				parts := strings.SplitN(strings.TrimPrefix(line, "STEP_STATUS: "), "=", 2)
				if len(parts) == 2 {
					step := parts[0]
					status := parts[1]
					log.Infof("K8s installation step %s: %s on node %s", step, status, i.nodeName)

					// If components are already installed, return success
					if step == "K8S_CHECK" && status == "already_installed" {
						log.Infof("Kubernetes components already installed on node %s, skipping", i.nodeName)
						return nil
					}

					// If any step failed, log it but continue with retry logic
					if status == "failure" && retry < maxRetries-1 {
						log.Infof("K8s installation step %s failed on node %s, will retry", step, i.nodeName)
					}
				}
			} else if strings.Contains(line, "already installed") {
				log.Infof("%s on node %s", line, i.nodeName)
			}
		}

		if err == nil {
			// Check if overall process was successful
			for _, line := range lines {
				if strings.HasPrefix(line, "STEP_STATUS: OVERALL=success") {
					log.Infof("Successfully installed Kubernetes components on node %s", i.nodeName)
					return nil
				}
			}
		}

		if retry < maxRetries-1 {
			log.Infof("Failed to install Kubernetes components on node %s, retrying in %v (%d/%d)...",
				i.nodeName, retryDelay, retry+1, maxRetries)
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to install Kubernetes components on node %s after %d attempts: %w", i.nodeName, maxRetries, err)
	}

	return fmt.Errorf("failed to install Kubernetes components on node %s after %d attempts", i.nodeName, maxRetries)
}

// InstallHelm installs Helm using binary files
func (i *Initializer) InstallHelm() error {
	// Create a script that handles the entire Helm installation process
	// This includes checking if Helm is installed, detecting architecture,
	// downloading the binary if needed, and installing it

	script := fmt.Sprintf(`#!/bin/bash
set -e

# Function to log steps and their status
log_step() {
    echo "STEP_STATUS: $1=$2"
}

# Step 1: Check if Helm is already installed
if command -v helm &> /dev/null; then
    HELM_VERSION=$(helm version --short 2>/dev/null | cut -d "+" -f 1)
    echo "Helm already installed: $HELM_VERSION"
    log_step "HELM_CHECK" "already_installed"
    exit 0
fi

# Step 2: Detect architecture
ARCH=%s
echo "Detected architecture: $ARCH"
log_step "ARCH_DETECTION" "success"

# Step 3: Set file paths
HELM_VERSION="%s"
HELM_PACKAGE="helm-$HELM_VERSION-linux-$ARCH.tar.gz"
HELM_PACKAGE_PATH="/usr/local/src/$HELM_PACKAGE"
DOWNLOAD_URL="https://get.helm.sh/$HELM_PACKAGE"

# Step 4: Check if the package already exists
if [ -f "$HELM_PACKAGE_PATH" ]; then
    echo "Helm package already exists at $HELM_PACKAGE_PATH"
    log_step "PACKAGE_CHECK" "exists"
else
    echo "Downloading Helm package from $DOWNLOAD_URL"
    sudo mkdir -p /usr/local/src
    sudo curl -sSL "$DOWNLOAD_URL" -o "$HELM_PACKAGE_PATH"
    if [ $? -eq 0 ]; then
        echo "Successfully downloaded Helm package to $HELM_PACKAGE_PATH"
        log_step "DOWNLOAD_HELM" "success"
    else
        echo "Failed to download Helm package"
        log_step "DOWNLOAD_HELM" "failure"
        exit 1
    fi
fi

# Step 5: Extract and install Helm
echo "Extracting and installing Helm"
sudo mkdir -p /tmp/helm-install
sudo tar -zxf "$HELM_PACKAGE_PATH" -C /tmp/helm-install
if [ $? -ne 0 ]; then
    echo "Failed to extract Helm package"
    log_step "EXTRACT_HELM" "failure"
    exit 1
fi
log_step "EXTRACT_HELM" "success"

# Step 6: Move binary to /usr/local/bin
sudo mv /tmp/helm-install/linux-$ARCH/helm /usr/local/bin/helm
if [ $? -ne 0 ]; then
    echo "Failed to move Helm binary to /usr/local/bin"
    log_step "INSTALL_HELM" "failure"
    exit 1
fi
sudo chmod +x /usr/local/bin/helm
log_step "INSTALL_HELM" "success"

# Step 7: Clean up
sudo rm -rf /tmp/helm-install
log_step "CLEANUP" "success"

# Step 8: Verify installation
INSTALLED_VERSION=$(helm version --short 2>/dev/null | cut -d "+" -f 1)
if [ -n "$INSTALLED_VERSION" ]; then
    echo "Successfully installed Helm $INSTALLED_VERSION"
    log_step "VERIFY_HELM" "success"
else
    echo "Failed to verify Helm installation"
    log_step "VERIFY_HELM" "failure"
    exit 1
fi

log_step "OVERALL" "success"
exit 0
`, i.arch, i.options.HelmVersion)

	// Execute the script in a single SSH connection
	log.Infof("Installing Helm on node %s", i.nodeName)

	// Set up retry logic
	maxRetries := 3
	retryDelay := 5 * time.Second
	var output string
	var err error

	for retry := 0; retry < maxRetries; retry++ {
		output, err = i.sshRunner.RunSSHCommand(i.nodeName, script)

		// Parse the output to check for any failures or if already installed
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "STEP_STATUS:") {
				parts := strings.SplitN(strings.TrimPrefix(line, "STEP_STATUS: "), "=", 2)
				if len(parts) == 2 {
					step := parts[0]
					status := parts[1]
					log.Infof("Helm installation step %s: %s on node %s", step, status, i.nodeName)

					// If Helm is already installed, return success
					if step == "HELM_CHECK" && status == "already_installed" {
						log.Infof("Helm already installed on node %s, skipping", i.nodeName)
						return nil
					}

					// If any step failed, log it but continue with retry logic
					if status == "failure" && retry < maxRetries-1 {
						log.Infof("Helm installation step %s failed on node %s, will retry", step, i.nodeName)
					}
				}
			} else if strings.Contains(line, "already installed") {
				log.Infof("%s on node %s", line, i.nodeName)
			}
		}

		if err == nil {
			// Check if overall process was successful
			for _, line := range lines {
				if strings.HasPrefix(line, "STEP_STATUS: OVERALL=success") {
					log.Infof("Successfully installed Helm on node %s", i.nodeName)
					return nil
				}
			}
		}

		if retry < maxRetries-1 {
			log.Infof("Failed to install Helm on node %s, retrying in %v (%d/%d)...",
				i.nodeName, retryDelay, retry+1, maxRetries)
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to install Helm on node %s after %d attempts: %w", i.nodeName, maxRetries, err)
	}

	return fmt.Errorf("failed to install Helm on node %s after %d attempts", i.nodeName, maxRetries)
}

// InstallZstd installs zstd package based on OS type
func (i *Initializer) InstallZstd() error {
	// Call appropriate install function based on OS type
	switch i.osType {
	case osTypeDebian:
		return i.installZstdOnDebian()
	case osTypeRedhat:
		return i.installZstdOnRedhat()
	default:
		log.Infof("Unknown OS type %s on node %s, defaulting to Debian", i.osType, i.nodeName)
		return fmt.Errorf("unsupported OS type: %s", i.osType)
	}
}

// installZstdOnDebian installs zstd on Debian-based systems
func (i *Initializer) installZstdOnDebian() error {
	cmd := "sudo apt-get install -y zstd"
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to install zstd on node %s: %w", i.nodeName, err)
	}
	log.Infof("Successfully installed zstd on node %s", i.nodeName)
	return nil
}

// installZstdOnRedhat installs zstd on RedHat-based systems
func (i *Initializer) installZstdOnRedhat() error {
	var cmd string
	if i.useDnf {
		// Use dnf if available
		log.Infof("Using dnf to install zstd on node %s", i.nodeName)
		cmd = "sudo dnf install -y zstd"
	} else {
		// Fall back to yum
		log.Infof("dnf not found, using yum to install zstd on node %s", i.nodeName)
		cmd = "sudo yum install -y zstd"
	}

	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to install zstd on node %s: %w", i.nodeName, err)
	}
	log.Infof("Successfully installed zstd on node %s", i.nodeName)
	return nil
}

// Initialize executes all initialization steps
func (i *Initializer) Initialize() error {
	if err := i.DoSystemUpdate(); err != nil {
		return err
	}

	// Install zstd immediately after system update
	if err := i.InstallZstd(); err != nil {
		return err
	}
	// Based on options, disable swap if specified
	if i.options.DisableSwap {
		if err := i.DisableSwap(); err != nil {
			return err
		}
	}

	// Based on options, enable IPVS if specified
	if i.options.EnableIPVS {
		if err := i.EnableIPVS(); err != nil {
			// Try to update package repositories and retry
			if err := i.AptUpdateForFixMissing(); err != nil {
				return err
			}
			if err := i.EnableIPVS(); err != nil {
				return err
			}
		}
	} else {
		// If not enabling IPVS, still need to set network bridging
		if err := i.EnableNetworkBridge(); err != nil {
			// Try to update package repositories and retry
			if err := i.AptUpdateForFixMissing(); err != nil {
				return err
			}
			if err := i.EnableNetworkBridge(); err != nil {
				return err
			}
		}
	}

	// Install container runtime
	if i.options.ContainerRuntime == "containerd" {
		if err := i.InstallContainerd(); err != nil {
			// Try to update package repositories and retry
			if err := i.AptUpdateForFixMissing(); err != nil {
				return err
			}
			if err := i.InstallContainerd(); err != nil {
				return err
			}
		}
	}

	// Install Kubernetes components
	if err := i.InstallK8sComponents(); err != nil {
		// Try to update package repositories and retry
		if err := i.AptUpdateForFixMissing(); err != nil {
			return err
		}
		if err := i.InstallK8sComponents(); err != nil {
			return err
		}
	}

	// Install Helm
	if err := i.InstallHelm(); err != nil {
		return err
	}

	return nil
}
