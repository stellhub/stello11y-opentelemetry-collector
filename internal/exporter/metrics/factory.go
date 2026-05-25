package metrics

import (
	"context"

	"github.com/stellhub/stello11y-opentelemetry-collector/internal/shared/datacount"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

const typeStr = "stellspec_metrics"

var typeID = component.MustNewType(typeStr)

type Config struct {
	Backend string `mapstructure:"backend"`
}

func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		typeID,
		createDefaultConfig,
		exporter.WithMetrics(createMetricsExporter, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	return &Config{Backend: "stellspec"}
}

func createMetricsExporter(ctx context.Context, set exporter.Settings, cfg component.Config) (exporter.Metrics, error) {
	typedCfg := cfg.(*Config)

	return exporterhelper.NewMetrics(
		ctx,
		set,
		cfg,
		func(_ context.Context, md pmetric.Metrics) error {
			return consumeMetrics(set, typedCfg, md)
		},
		exporterhelper.WithStart(func(context.Context, component.Host) error {
			set.Logger.Info("stellspec metrics exporter started", zap.String("exporter", typeStr), zap.String("backend", typedCfg.Backend))
			return nil
		}),
		exporterhelper.WithShutdown(func(context.Context) error {
			set.Logger.Info("stellspec metrics exporter stopped", zap.String("exporter", typeStr), zap.String("backend", typedCfg.Backend))
			return nil
		}),
	)
}

func consumeMetrics(set exporter.Settings, cfg *Config, md pmetric.Metrics) error {
	set.Logger.Info(
		"stellspec metrics exporter consumed batch",
		zap.String("exporter", typeStr),
		zap.String("backend", cfg.Backend),
		zap.Int("resource_metrics", md.ResourceMetrics().Len()),
		zap.Int("metric_data_points", datacount.MetricDataPoints(md)),
	)
	return nil
}
