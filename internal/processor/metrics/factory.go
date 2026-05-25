package metrics

import (
	"context"

	"github.com/stellhub/stello11y-opentelemetry-collector/internal/processor/stellspec"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

const typeStr = "stellspec_metrics"

var typeID = component.MustNewType(typeStr)

type Config struct {
	*stellspec.Config `mapstructure:",squash"`
}

type metricsProcessor struct {
	component.StartFunc
	component.ShutdownFunc
	consumer.Metrics
}

func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeID,
		createDefaultConfig,
		processor.WithMetrics(createMetricsProcessor, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	return &Config{Config: stellspec.NewDefaultConfig(stellspec.SignalMetrics)}
}

func createMetricsProcessor(_ context.Context, set processor.Settings, cfg component.Config, next consumer.Metrics) (processor.Metrics, error) {
	typedCfg := cfg.(*Config)
	runtime := stellspec.NewRuntime(typedCfg.Config)
	metricsConsumer, err := consumer.NewMetrics(func(ctx context.Context, md pmetric.Metrics) error {
		runtime.ProcessMetrics(md)
		return next.ConsumeMetrics(ctx, md)
	}, consumer.WithCapabilities(consumer.Capabilities{MutatesData: true}))
	if err != nil {
		return nil, err
	}

	return &metricsProcessor{
		StartFunc: func(context.Context, component.Host) error {
			set.Logger.Info("stellspec metrics processor started", zap.String("processor", typeStr))
			return nil
		},
		ShutdownFunc: func(context.Context) error {
			set.Logger.Info("stellspec metrics processor stopped", zap.String("processor", typeStr))
			return nil
		},
		Metrics: metricsConsumer,
	}, nil
}
