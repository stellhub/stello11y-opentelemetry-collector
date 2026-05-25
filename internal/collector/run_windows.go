//go:build windows

package collector

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"

	"go.opentelemetry.io/collector/otelcol"
)

func run(params otelcol.CollectorSettings) error {
	if err := svc.Run("", otelcol.NewSvcHandler(params)); err != nil {
		if errors.Is(err, windows.ERROR_FAILED_SERVICE_CONTROLLER_CONNECT) {
			return runInteractive(params)
		}
		return fmt.Errorf("failed to start collector service: %w", err)
	}

	return nil
}
