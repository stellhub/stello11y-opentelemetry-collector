//go:build !windows

package collector

import "go.opentelemetry.io/collector/otelcol"

func run(params otelcol.CollectorSettings) error {
	return runInteractive(params)
}
