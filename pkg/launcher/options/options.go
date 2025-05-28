package options

// Config contains common configuration options for launchers
type Options struct {
	Prefix       string
	Template     string
	Password     string
	SSHKey       string
	SSHPubKey    string
	OutputFormat string
	Parallel     int
	Force        bool
}
