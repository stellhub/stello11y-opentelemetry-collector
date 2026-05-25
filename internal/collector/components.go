package collector

import (
	deltatocumulativeprocessor "github.com/open-telemetry/opentelemetry-collector-contrib/processor/deltatocumulativeprocessor"
	logsexporter "github.com/stellhub/stello11y-opentelemetry-collector/internal/exporter/logs"
	metricsexporter "github.com/stellhub/stello11y-opentelemetry-collector/internal/exporter/metrics"
	tracesexporter "github.com/stellhub/stello11y-opentelemetry-collector/internal/exporter/traces"
	logsprocessor "github.com/stellhub/stello11y-opentelemetry-collector/internal/processor/logs"
	metricsprocessor "github.com/stellhub/stello11y-opentelemetry-collector/internal/processor/metrics"
	tracesprocessor "github.com/stellhub/stello11y-opentelemetry-collector/internal/processor/traces"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	otelconftelemetry "go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"
)

type aliasProvider interface{ DeprecatedAlias() component.Type }

func makeModulesMap[T component.Factory](factories map[component.Type]T, modules map[component.Type]string) map[component.Type]string {
	for compType, factory := range factories {
		if ap, ok := any(factory).(aliasProvider); ok {
			alias := ap.DeprecatedAlias()
			if alias.String() != "" {
				modules[alias] = modules[compType]
			}
		}
	}
	return modules
}

func components() (otelcol.Factories, error) {
	var err error

	factories := otelcol.Factories{
		Telemetry: otelconftelemetry.NewFactory(),
	}

	factories.Extensions, err = otelcol.MakeFactoryMap[extension.Factory]()
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.ExtensionModules = makeModulesMap(factories.Extensions, map[component.Type]string{})

	factories.Receivers, err = otelcol.MakeFactoryMap[receiver.Factory](
		otlpreceiver.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.ReceiverModules = makeModulesMap(factories.Receivers, map[component.Type]string{
		otlpreceiver.NewFactory().Type(): "go.opentelemetry.io/collector/receiver/otlpreceiver v0.151.0",
	})

	factories.Processors, err = otelcol.MakeFactoryMap[processor.Factory](
		logsprocessor.NewFactory(),
		metricsprocessor.NewFactory(),
		tracesprocessor.NewFactory(),
		deltatocumulativeprocessor.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.ProcessorModules = makeModulesMap(factories.Processors, map[component.Type]string{
		logsprocessor.NewFactory().Type():              "github.com/stellhub/stello11y-opentelemetry-collector/internal/processor/logs",
		metricsprocessor.NewFactory().Type():           "github.com/stellhub/stello11y-opentelemetry-collector/internal/processor/metrics",
		tracesprocessor.NewFactory().Type():            "github.com/stellhub/stello11y-opentelemetry-collector/internal/processor/traces",
		deltatocumulativeprocessor.NewFactory().Type(): "github.com/open-telemetry/opentelemetry-collector-contrib/processor/deltatocumulativeprocessor v0.151.0",
	})

	factories.Exporters, err = otelcol.MakeFactoryMap[exporter.Factory](
		logsexporter.NewFactory(),
		metricsexporter.NewFactory(),
		tracesexporter.NewFactory(),
		otlpexporter.NewFactory(),
		otlphttpexporter.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.ExporterModules = makeModulesMap(factories.Exporters, map[component.Type]string{
		logsexporter.NewFactory().Type():     "github.com/stellhub/stello11y-opentelemetry-collector/internal/exporter/logs",
		metricsexporter.NewFactory().Type():  "github.com/stellhub/stello11y-opentelemetry-collector/internal/exporter/metrics",
		tracesexporter.NewFactory().Type():   "github.com/stellhub/stello11y-opentelemetry-collector/internal/exporter/traces",
		otlpexporter.NewFactory().Type():     "go.opentelemetry.io/collector/exporter/otlpexporter v0.151.0",
		otlphttpexporter.NewFactory().Type(): "go.opentelemetry.io/collector/exporter/otlphttpexporter v0.151.0",
	})

	factories.Connectors, err = otelcol.MakeFactoryMap[connector.Factory]()
	if err != nil {
		return otelcol.Factories{}, err
	}
	factories.ConnectorModules = makeModulesMap(factories.Connectors, map[component.Type]string{})

	return factories, nil
}
