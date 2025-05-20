package cni

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/monshunter/ohmykube/pkg/clusterinfo"
	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// FlannelInstaller is responsible for installing Flannel CNI
type FlannelInstaller struct {
	SSHClient  *ssh.Client
	MasterNode string
	PodCIDR    string
	Version    string
}

// NewFlannelInstaller creates a Flannel installer
func NewFlannelInstaller(sshClient *ssh.Client, masterNode string) *FlannelInstaller {
	return &FlannelInstaller{
		SSHClient:  sshClient,
		MasterNode: masterNode,
		PodCIDR:    "10.244.0.0/16", // Flannel default Pod CIDR
		Version:    "v0.26.7",       // Flannel version
	}
}

// Install installs Flannel CNI
func (f *FlannelInstaller) Install() error {
	ymlURL := fmt.Sprintf("https://github.com/flannel-io/flannel/releases/download/%s/kube-flannel.yml", f.Version)
	// Ensure br_netfilter module is loaded
	brNetfilterCmd := `
sudo modprobe br_netfilter
echo "br_netfilter" | sudo tee /etc/modules-load.d/br_netfilter.conf
`
	_, err := f.SSHClient.RunCommand(brNetfilterCmd)
	if err != nil {
		return fmt.Errorf("failed to load br_netfilter module: %w", err)
	}

	// Use ClusterInfo to get the cluster's Pod CIDR
	clusterInfo := clusterinfo.NewClusterInfo(f.SSHClient)
	podCIDR, err := clusterInfo.GetPodCIDR()
	if err == nil && podCIDR != "" {
		f.PodCIDR = podCIDR
		log.Infof("Retrieved cluster Pod CIDR: %s", f.PodCIDR)
	}

	// Step 1: Download the flannel manifest locally
	log.Info("Downloading Flannel manifest")
	resp, err := http.Get(ymlURL)
	if err != nil {
		return fmt.Errorf("failed to download Flannel manifest: %w", err)
	}
	defer resp.Body.Close()

	// Step 2: Read and modify the YAML content
	yamlContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read Flannel manifest: %w", err)
	}

	// Replace the default Pod CIDR with the specified one
	log.Infof("Replacing default Pod CIDR with %s", f.PodCIDR)
	oldCIDR := `"Network": "10.244.0.0/16"`
	newCIDR := fmt.Sprintf(`"Network": "%s"`, f.PodCIDR)
	modifiedYaml := strings.Replace(string(yamlContent), oldCIDR, newCIDR, -1)

	// Step 3: Save the modified YAML to a temporary file and transfer to the remote server
	localTempFile, err := os.CreateTemp("", "kube-flannel-*.yml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(localTempFile.Name())

	if _, err = localTempFile.WriteString(modifiedYaml); err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	localTempFile.Close()

	// Upload the modified file to the remote server
	remoteTempFile := "/tmp/kube-flannel.yml"
	log.Info("Transferring modified Flannel manifest to remote server")
	if err = f.SSHClient.TransferFile(localTempFile.Name(), remoteTempFile); err != nil {
		return fmt.Errorf("failed to transfer file to remote server: %w", err)
	}

	// Step 4: Apply the modified YAML on the remote server
	log.Info("Applying Flannel manifest on the remote server")
	applyCmd := fmt.Sprintf("kubectl apply -f %s", remoteTempFile)
	_, err = f.SSHClient.RunCommand(applyCmd)
	if err != nil {
		return fmt.Errorf("failed to apply Flannel manifest: %w", err)
	}

	// Wait for Flannel to be ready
	waitCmd := `kubectl -n kube-flannel wait --for=condition=ready pod -l app=flannel --timeout=120s`
	_, err = f.SSHClient.RunCommand(waitCmd)
	if err != nil {
		log.Infof("Warning: Timed out waiting for Flannel to be ready, but will continue: %v", err)
	}

	return nil
}

// SetPodCIDR sets the Pod CIDR
func (f *FlannelInstaller) SetPodCIDR(cidr string) {
	f.PodCIDR = cidr
}

// SetVersion sets the Flannel version
func (f *FlannelInstaller) SetVersion(version string) {
	f.Version = version
}
