package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version information variables
var (
	version   = "dev"
	buildTime = "unknown"
	goVersion = "unknown"
)

// SetVersionInfo sets the version information from main package
func SetVersionInfo(v, bt, gv string) {
	version = v
	buildTime = bt
	goVersion = gv
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Long:  `Print the version information of OhMyKube including version, build time, and Go version.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("OhMyKube version: %s\n", version)
		fmt.Printf("Build time: %s\n", buildTime)
		fmt.Printf("Go version: %s\n", goVersion)
	},
}
