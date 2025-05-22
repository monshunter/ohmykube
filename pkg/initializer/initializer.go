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

// Initializer used to initialize a single Kubernetes node environment
type Initializer struct {
	sshRunner SSHCommandRunner
	nodeName  string
	options   InitOptions
}

// NewInitializer creates a new node initializer
func NewInitializer(sshRunner SSHCommandRunner, nodeName string) *Initializer {
	return &Initializer{
		sshRunner: sshRunner,
		nodeName:  nodeName,
		options:   DefaultInitOptions(),
	}
}

// NewInitializerWithOptions creates a new node initializer with specified options
func NewInitializerWithOptions(sshRunner SSHCommandRunner, nodeName string, options InitOptions) *Initializer {
	return &Initializer{
		sshRunner: sshRunner,
		nodeName:  nodeName,
		options:   options,
	}
}

// waitForAptLock waits for apt lock to be released
func (i *Initializer) waitForAptLock() error {
	maxRetries := 30          // maximum 30 retries
	retryDelay := time.Second // wait 1 second each time

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Check apt lock status
		cmd := `if fuser /var/lib/apt/lists/lock /var/lib/dpkg/lock-frontend /var/lib/dpkg/lock 2>/dev/null; then
	echo "locked"
else
	echo "unlocked"
fi`

		output, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
		if err != nil {
			return fmt.Errorf("failed to check apt lock status: %w", err)
		}

		// If no longer locked, continue
		if attempt > 1 && strings.TrimSpace(output) == "unlocked" {
			log.Infof("Apt lock released on node %s, continuing installation", i.nodeName)
			return nil
		}

		// If still locked, wait a while and try again
		if attempt < maxRetries {
			log.Infof("Apt still locked on node %s, waiting for release (attempt %d/%d)...", i.nodeName, attempt, maxRetries)
			time.Sleep(retryDelay)
		} else {
			// If max retries reached, attempt to forcefully release the lock
			log.Infof("Timed out waiting for apt lock release on node %s, attempting to force release...", i.nodeName)

			killCmd := "sudo killall apt apt-get dpkg 2>/dev/null || true"
			_, err = i.sshRunner.RunSSHCommand(i.nodeName, killCmd)
			if err != nil {
				// Ignore potential errors from killall as processes may not exist
			}

			unlockCmd := `sudo rm -f /var/lib/apt/lists/lock
						  sudo rm -f /var/cache/apt/archives/lock
						  sudo rm -f /var/lib/dpkg/lock
						  sudo rm -f /var/lib/dpkg/lock-frontend
						  sudo dpkg --configure -a`

			_, err = i.sshRunner.RunSSHCommand(i.nodeName, unlockCmd)
			if err != nil {
				return fmt.Errorf("failed to force release apt lock: %w", err)
			}

			log.Infof("Apt lock forcefully released on node %s", i.nodeName)

			// Fix: wait an extra period after force releasing the lock to ensure it's truly released
			extraWaitTime := 10 * time.Second
			log.Infof("Waiting an extra %s on node %s to ensure lock is fully released...", extraWaitTime, i.nodeName)
			time.Sleep(extraWaitTime)

			// Check lock status again
			output, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
			if err != nil {
				return fmt.Errorf("failed to check apt lock status after force release: %w", err)
			}

			if strings.TrimSpace(output) != "unlocked" {
				return fmt.Errorf("apt lock still locked on node %s after force release", i.nodeName)
			}

			log.Infof("Confirmed apt lock fully released on node %s", i.nodeName)
			return nil
		}
	}

	return fmt.Errorf("timed out waiting for apt lock release")
}

func (i *Initializer) AptUpdate() error {
	// Update apt
	cmd := "sudo apt-get update"
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to update apt on node %s: %w", i.nodeName, err)
	}
	return nil
}

func (i *Initializer) AptUpdateForFixMissing() error {
	// Update apt
	cmd := "sudo apt-get update --fix-missing -y"
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to update apt on node %s: %w", i.nodeName, err)
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
	// Create /etc/modules-load.d/k8s.conf file
	modulesFile := "/etc/modules-load.d/k8s.conf"
	cmd := fmt.Sprintf("cat <<EOF | sudo tee %s\n%sEOF", modulesFile, ipvs.K8S_MODULES_CONFIG)
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to create %s file on node %s: %w", modulesFile, i.nodeName, err)
	}

	// Load kernel modules
	modules := []string{
		"overlay",
		"br_netfilter",
		"ip_vs",
		"ip_vs_rr",
		"ip_vs_wrr",
		"ip_vs_sh",
		"nf_conntrack",
	}

	for _, module := range modules {
		cmd := fmt.Sprintf("sudo modprobe %s", module)
		_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
		if err != nil {
			return fmt.Errorf("failed to load %s module on node %s: %w", module, i.nodeName, err)
		}
	}

	// Wait for apt lock release
	if err := i.waitForAptLock(); err != nil {
		return fmt.Errorf("failed to install IPVS tools on node %s: %w", i.nodeName, err)
	}

	// Install IPVS tools
	cmd = "sudo apt-get install -y ipvsadm ipset"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to install IPVS tools on node %s: %w", i.nodeName, err)
	}

	// Set system parameters
	sysctlFile := "/etc/sysctl.d/k8s.conf"
	cmd = fmt.Sprintf("cat <<EOF | sudo tee %s\n%sEOF", sysctlFile, ipvs.K8S_SYSCTL_CONFIG)
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to create %s file on node %s: %w", sysctlFile, i.nodeName, err)
	}

	// Apply system parameters
	cmd = "sudo sysctl --system"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to apply system parameters on node %s: %w", i.nodeName, err)
	}

	return nil
}

// EnableNetworkBridge enables network bridging
func (i *Initializer) EnableNetworkBridge() error {
	// Create /etc/modules-load.d/k8s.conf file
	modulesFile := "/etc/modules-load.d/k8s.conf"
	modulesContent := "overlay\nbr_netfilter\n"
	cmd := fmt.Sprintf("cat <<EOF | sudo tee %s\n%sEOF", modulesFile, modulesContent)
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to create %s file on node %s: %w", modulesFile, i.nodeName, err)
	}

	// Load basic modules
	modules := []string{
		"overlay",
		"br_netfilter",
	}

	for _, module := range modules {
		cmd := fmt.Sprintf("sudo modprobe %s", module)
		_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
		if err != nil {
			return fmt.Errorf("failed to load %s module on node %s: %w", module, i.nodeName, err)
		}
	}

	// Set basic system parameters
	sysctlFile := "/etc/sysctl.d/k8s.conf"
	sysctlContent := `net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
`
	cmd = fmt.Sprintf("cat <<EOF | sudo tee %s\n%sEOF", sysctlFile, sysctlContent)
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to create %s file on node %s: %w", sysctlFile, i.nodeName, err)
	}

	// Apply system parameters
	cmd = "sudo sysctl --system"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to apply system parameters on node %s: %w", i.nodeName, err)
	}

	return nil
}

// InstallContainerd installs and configures containerd
func (i *Initializer) InstallContainerd() error {
	// Wait for apt lock release
	if err := i.waitForAptLock(); err != nil {
		return fmt.Errorf("failed to install containerd on node %s: %w", i.nodeName, err)
	}

	// Install containerd
	cmd := "sudo apt-get install -y containerd"
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to install containerd on node %s: %w", i.nodeName, err)
	}

	// Create containerd configuration directory
	cmd = "sudo mkdir -p /etc/containerd"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to create containerd configuration directory on node %s: %w", i.nodeName, err)
	}

	cmd = "sudo mkdir -p /etc/containerd/certs.d"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to create containerd certificate directory on node %s: %w", i.nodeName, err)
	}

	// Set mirrors
	if envar.IsEnableDefaultMiror() {
		for _, mirror := range containerd.Mirrors() {
			cmd = fmt.Sprintf("sudo mkdir -p /etc/containerd/certs.d/%s", mirror.Name)
			_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
			if err != nil {
				return fmt.Errorf("failed to create containerd certificate directory on node %s: %w", i.nodeName, err)
			}
			cmd = fmt.Sprintf("sudo tee /etc/containerd/certs.d/%s/hosts.toml <<EOF\n%sEOF", mirror.Name, mirror.Config)
			_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
			if err != nil {
				return fmt.Errorf("failed to create containerd certificate directory on node %s: %w", i.nodeName, err)
			}
		}
	}

	// Write default configuration
	configFile := "/etc/containerd/config.toml"
	cmd = fmt.Sprintf("cat <<EOF | sudo tee %s\n%sEOF", configFile, containerd.CONFIG)
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to create containerd configuration file on node %s: %w", i.nodeName, err)
	}

	// Restart containerd
	cmd = "sudo systemctl restart containerd"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to restart containerd on node %s: %w", i.nodeName, err)
	}

	// Enable containerd auto-start on boot
	cmd = "sudo systemctl enable containerd"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to enable containerd auto-start on node %s: %w", i.nodeName, err)
	}

	return nil
}

// InstallK8sComponents installs kubeadm, kubectl, kubelet
func (i *Initializer) InstallK8sComponents() error {
	// Wait for apt lock release
	if err := i.waitForAptLock(); err != nil {
		return fmt.Errorf("failed to update apt on node %s: %w", i.nodeName, err)
	}

	// Install dependencies
	cmd := "sudo apt-get install -y apt-transport-https ca-certificates curl gpg"
	_, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to install dependencies on node %s: %w", i.nodeName, err)
	}

	// Create certificate directory
	cmd = "sudo mkdir -p -m 755 /etc/apt/keyrings"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to create certificate directory on node %s: %w", i.nodeName, err)
	}

	// Download k8s public key and import (add retry mechanism)
	// Use configured K8sMirrorURL or default value
	mirrorURL := i.options.K8sMirrorURL
	keyURL := fmt.Sprintf("%s/Release.key", mirrorURL)

	// Add retry mechanism
	maxRetries := 3
	retryDelay := 5 * time.Second
	var downloadErr error

	for retry := 0; retry < maxRetries; retry++ {
		downloadKeyCmd := fmt.Sprintf("curl -fsSL %s | sudo gpg --batch --yes --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg", keyURL)
		_, downloadErr = i.sshRunner.RunSSHCommand(i.nodeName, downloadKeyCmd)

		if downloadErr == nil {
			// Successfully downloaded and imported key
			break
		}

		if retry < maxRetries-1 {
			log.Infof("Failed to download and import K8s key on node %s, retrying in %v (%d/%d)...",
				i.nodeName, retryDelay, retry+1, maxRetries)
			time.Sleep(retryDelay)
		}
	}

	if downloadErr != nil {
		return fmt.Errorf("failed to download and import K8s key on node %s: %w", i.nodeName, downloadErr)
	}

	// Add k8s source
	repoURL := fmt.Sprintf("deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] %s/ /", mirrorURL)
	cmd = fmt.Sprintf("echo '%s' | sudo tee /etc/apt/sources.list.d/kubernetes.list", repoURL)
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to add Kubernetes source on node %s: %w", i.nodeName, err)
	}

	// Wait for apt lock release
	if err := i.waitForAptLock(); err != nil {
		return fmt.Errorf("failed to update apt on node %s: %w", i.nodeName, err)
	}
	// Install k8s components (add retry mechanism)
	maxRetries = 3
	for retry := 0; retry < maxRetries; retry++ {
		cmd = "sudo apt-get install -y kubelet kubeadm kubectl"
		_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)

		if err == nil {
			break
		}

		if retry < maxRetries-1 {
			log.Infof("Failed to install Kubernetes components on node %s, retrying in %v (%d/%d)...",
				i.nodeName, retryDelay, retry+1, maxRetries)
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to install Kubernetes components on node %s: %w", i.nodeName, err)
	}

	// Lock version
	cmd = "sudo apt-mark hold kubelet kubeadm kubectl"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to lock Kubernetes components version on node %s: %w", i.nodeName, err)
	}

	// Enable kubelet
	cmd = "sudo systemctl enable --now kubelet"
	_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err != nil {
		return fmt.Errorf("failed to enable kubelet on node %s: %w", i.nodeName, err)
	}

	return nil
}

// InstallHelm installs Helm
func (i *Initializer) InstallHelm() error {
	// Check if Helm is already installed
	cmd := "command -v helm && echo 'Helm installed' || echo 'Helm not installed'"
	output, err := i.sshRunner.RunSSHCommand(i.nodeName, cmd)
	if err == nil && output == "Helm installed" {
		log.Infof("Helm already installed on node %s, skipping", i.nodeName)
		return nil
	}

	// Install Helm (add retry mechanism)
	maxRetries := 3
	retryDelay := 5 * time.Second

	for retry := 0; retry < maxRetries; retry++ {
		cmd = `curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash`
		_, err = i.sshRunner.RunSSHCommand(i.nodeName, cmd)

		if err == nil {
			break
		}

		if retry < maxRetries-1 {
			log.Infof("Failed to install Helm on node %s, retrying in %v (%d/%d)...",
				i.nodeName, retryDelay, retry+1, maxRetries)
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to install Helm on node %s: %w", i.nodeName, err)
	}

	log.Infof("Successfully installed Helm on node %s", i.nodeName)
	return nil
}

// Initialize executes all initialization steps
func (i *Initializer) Initialize() error {
	if err := i.AptUpdate(); err != nil {
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
			return err
		}
	}

	// Install container runtime
	if i.options.ContainerRuntime == "containerd" {
		if err := i.InstallContainerd(); err != nil {
			if err := i.AptUpdateForFixMissing(); err != nil {
				return err
			}
			if err := i.InstallContainerd(); err != nil {
				return err
			}
		}
	}

	if err := i.InstallK8sComponents(); err != nil {
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
