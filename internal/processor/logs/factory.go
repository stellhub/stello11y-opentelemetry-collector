package logs

import (
	"context"

	"github.com/stellhub/stello11y-opentelemetry-collector/internal/processor/stellspec"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

const typeStr = "stellspec_logs"

var typeID = component.MustNewType(typeStr)

type Config struct {
	*stellspec.Config `mapstructure:",squash"`
}

type logsProcessor struct {
	component.StartFunc
	component.ShutdownFunc
	consumer.Logs
}

func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeID,
		createDefaultConfig,
		processor.WithLogs(createLogsProcessor, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	return &Config{Config: stellspec.NewDefaultConfig(stellspec.SignalLogs)}
}

func createLogsProcessor(_ context.Context, set processor.Settings, cfg component.Config, next consumer.Logs) (processor.Logs, error) {
	typedCfg := cfg.(*Config)
	runtime := stellspec.NewRuntime(typedCfg.Config)
	logsConsumer, err := consumer.NewLogs(func(ctx context.Context, ld plog.Logs) error {
		runtime.ProcessLogs(ld)
		return next.ConsumeLogs(ctx, ld)
	}, consumer.WithCapabilities(consumer.Capabilities{MutatesData: true}))
	if err != nil {
		return nil, err
	}

	return &logsProcessor{
		StartFunc: func(context.Context, component.Host) error {
			set.Logger.Info("stellspec logs processor started", zap.String("processor", typeStr))
			return nil
		},
		ShutdownFunc: func(context.Context) error {
			set.Logger.Info("stellspec logs processor stopped", zap.String("processor", typeStr))
			return nil
		},
		Logs: logsConsumer,
	}, nil
}
