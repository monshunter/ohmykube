package provider

// Provider defines the interface for virtual machine provider implementations
type Provider interface {
	Name() string

	// Template returns the template name or file path
	Template() string

	// Create creates a new virtual machine
	Create(name string, args ...any) error

	// Delete deletes a virtual machine
	Delete(name string) error

	// Start starts a virtual machine
	Start(name string) error

	// Stop stops a virtual machine
	Stop(name string) error

	// Shell opens an interactive shell to the virtual machine
	Shell(name string) error

	// Info gets information about a VM
	Info(name string) error

	// Exec executes a command on the specified virtual machine
	Exec(vmName, command string) (string, error)

	// List lists all virtual machines with the given prefix
	List() ([]string, error)

	// GetAddress gets the IP address of a node
	GetAddress(name string) (string, error)
}
