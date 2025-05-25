package interfaces

// SSHRunner defines the comprehensive SSH interface with three essential methods
type SSHRunner interface {
	SSHCommandRunner
	SSHFileTransfer
}

// SSHCommandRunner defines the interface for executing SSH commands (for backward compatibility)
type SSHCommandRunner interface {
	// Execute ordinary commands
	RunCommand(nodeName string, command string) (string, error)
}

// SSHFileTransfer defines the interface for transferring files over SSH (for backward compatibility)
type SSHFileTransfer interface {
	// Upload files (local to remote)
	UploadFile(nodeName string, localPath, remotePath string) error

	// Download files (remote to local)
	DownloadFile(nodeName string, remotePath, localPath string) error
}
