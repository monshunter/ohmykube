package interfaces

// ImageRecorder defines the interface for recording images
type ImageRecorder interface {
	// RecordImage records an image
	RecordImage(imageName string)

	// GetImageNames returns all recorded image names
	GetImageNames() []string

	// GetWorkerNames returns all worker nodes
	GetWorkerNames() []string

	// GetMasterName returns the master node
	GetMasterName() string
}
