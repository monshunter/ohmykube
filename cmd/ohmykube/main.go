package main

import (
	"github.com/monshunter/ohmykube/cmd/ohmykube/app"
	"github.com/monshunter/ohmykube/pkg/log"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatalf("Error: %s", err)
	}
}
