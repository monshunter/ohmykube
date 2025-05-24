package interfaces

// SSHRunner defines the comprehensive SSH interface with three essential methods
type SSHRunner interface {
	// 1. Execute ordinary commands
	RunCommand(nodeName string, command string) (string, error)

	// 2. Upload files (local to remote)
	UploadFile(nodeName string, localPath, remotePath string) error

	// 3. Download files (remote to local)
	DownloadFile(nodeName string, remotePath, localPath string) error
}

// SSHCommandRunner defines the interface for executing SSH commands (for backward compatibility)
type SSHCommandRunner interface {
	RunSSHCommand(nodeName string, command string) (string, error)
}

// SSHFileTransfer defines the interface for transferring files over SSH (for backward compatibility)
type SSHFileTransfer interface {
	TransferFile(nodeName string, localPath, remotePath string) error
	DownloadFile(nodeName string, remotePath, localPath string) error
}
