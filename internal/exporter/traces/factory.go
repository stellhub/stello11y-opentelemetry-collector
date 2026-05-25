package traces

import (
	"context"

	"github.com/stellhub/stello11y-opentelemetry-collector/internal/shared/datacount"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

const typeStr = "stellspec_traces"

var typeID = component.MustNewType(typeStr)

type Config struct {
	Backend string `mapstructure:"backend"`
}

func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		typeID,
		createDefaultConfig,
		exporter.WithTraces(createTracesExporter, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	return &Config{Backend: "stellspec"}
}

func createTracesExporter(ctx context.Context, set exporter.Settings, cfg component.Config) (exporter.Traces, error) {
	typedCfg := cfg.(*Config)

	return exporterhelper.NewTraces(
		ctx,
		set,
		cfg,
		func(_ context.Context, td ptrace.Traces) error {
			return consumeTraces(set, typedCfg, td)
		},
		exporterhelper.WithStart(func(context.Context, component.Host) error {
			set.Logger.Info("stellspec traces exporter started", zap.String("exporter", typeStr), zap.String("backend", typedCfg.Backend))
			return nil
		}),
		exporterhelper.WithShutdown(func(context.Context) error {
			set.Logger.Info("stellspec traces exporter stopped", zap.String("exporter", typeStr), zap.String("backend", typedCfg.Backend))
			return nil
		}),
	)
}

func consumeTraces(set exporter.Settings, cfg *Config, td ptrace.Traces) error {
	set.Logger.Info(
		"stellspec traces exporter consumed batch",
		zap.String("exporter", typeStr),
		zap.String("backend", cfg.Backend),
		zap.Int("resource_spans", td.ResourceSpans().Len()),
		zap.Int("spans", datacount.Spans(td)),
	)
	return nil
}
