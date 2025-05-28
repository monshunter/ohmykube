package main

import (
	"github.com/monshunter/ohmykube/cmd/ohmykube/app"
	"github.com/monshunter/ohmykube/pkg/log"
)

// Version information set by build-time LDFLAGS
var (
	Version   = "dev"
	BuildTime = "unknown"
	GoVersion = "unknown"
)

func main() {
	// Set version information for the app package
	app.SetVersionInfo(Version, BuildTime, GoVersion)

	if err := app.Run(); err != nil {
		log.Fatalf("Error: %s", err)
	}
}
