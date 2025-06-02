package types

type Status string

const (
	StatusRunning Status = "Running"
	StatusStopped Status = "Stopped"
	StatusUnknown Status = "Unknown"
	StatusError   Status = "Error"
)

func (status Status) String() string {
	return string(status)
}

func (status Status) IsRunning() bool {
	return status == StatusRunning
}

func (status Status) IsStopped() bool {
	return status == StatusStopped
}

func (status Status) IsUnknown() bool {
	return status == StatusUnknown
}

func (status Status) IsError() bool {
	return status == StatusError
}
