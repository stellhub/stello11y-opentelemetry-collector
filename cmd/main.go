package main

import (
	"os"

	"github.com/stellhub/stello11y-opentelemetry-collector/internal/collector"
)

func main() {
	if err := collector.Run(); err != nil {
		os.Exit(1)
	}
}
