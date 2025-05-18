package launcher

// VMInfo represents information about a virtual machine
type VMInfo struct {
	Name   string
	Status string
	IP     string
}

// Launcher defines the interface for virtual machine launcher implementations
type Launcher interface {
	// CreateVM creates a new virtual machine
	CreateVM(name string, cpus, memory, disk int) error

	// DeleteVM deletes a virtual machine
	DeleteVM(name string) error

	// InfoVM gets information about a VM
	InfoVM(name string) error

	// InitAuth initializes authentication for the VM
	InitAuth(name string) error

	// ExecCommand executes a command on the specified virtual machine
	ExecCommand(vmName, command string) (string, error)

	// ListVM lists all virtual machines with the given prefix
	ListVM(prefix string) ([]string, error)

	// TransferFile transfers a local file to the virtual machine
	TransferFile(localPath, vmName, remotePath string) error

	// GetNodeIP gets the IP address of a node
	GetNodeIP(nodeName string) (string, error)
}
