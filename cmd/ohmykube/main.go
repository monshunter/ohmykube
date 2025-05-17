package main

import (
	"github.com/monshunter/ohmykube/cmd/ohmykube/cmd"
	"github.com/monshunter/ohmykube/pkg/log"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatalf("Error: %s", err)
	}
}
