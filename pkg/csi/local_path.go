package csi

import (
	"fmt"

	"github.com/monshunter/ohmykube/pkg/log"
	"github.com/monshunter/ohmykube/pkg/ssh"
)

// LocalPathInstaller is responsible for installing local-path-provisioner CSI
type LocalPathInstaller struct {
	SSHClient  *ssh.Client
	MasterNode string
	Version    string
}

// NewLocalPathInstaller creates a local-path-provisioner installer
func NewLocalPathInstaller(sshClient *ssh.Client, masterNode string) *LocalPathInstaller {
	return &LocalPathInstaller{
		SSHClient:  sshClient,
		MasterNode: masterNode,
		Version:    "v0.0.31", // local-path-provisioner version
	}
}

// Install installs local-path-provisioner
func (l *LocalPathInstaller) Install() error {
	// Install local-path-provisioner
	log.Infof("Installing local-path-provisioner version %s...", l.Version)
	localPathCmd := fmt.Sprintf(`
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/%s/deploy/local-path-storage.yaml
`, l.Version)

	_, err := l.SSHClient.RunCommand(localPathCmd)
	if err != nil {
		return fmt.Errorf("failed to install local-path-provisioner: %w", err)
	}

	// Set local-path as default storage class
	log.Info("Setting local-path as default storage class...")
	defaultStorageClassCmd := `
kubectl patch storageclass local-path -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
`
	_, err = l.SSHClient.RunCommand(defaultStorageClassCmd)
	if err != nil {
		return fmt.Errorf("failed to set local-path as default storage class: %w", err)
	}

	log.Info("Local-path-provisioner installation completed successfully")
	return nil
}
