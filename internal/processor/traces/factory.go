package traces

import (
	"context"

	"github.com/stellhub/stello11y-opentelemetry-collector/internal/processor/stellspec"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

const typeStr = "stellspec_traces"

var typeID = component.MustNewType(typeStr)

type Config struct {
	*stellspec.Config `mapstructure:",squash"`
}

type tracesProcessor struct {
	component.StartFunc
	component.ShutdownFunc
	consumer.Traces
}

func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeID,
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	return &Config{Config: stellspec.NewDefaultConfig(stellspec.SignalTraces)}
}

func createTracesProcessor(_ context.Context, set processor.Settings, cfg component.Config, next consumer.Traces) (processor.Traces, error) {
	typedCfg := cfg.(*Config)
	runtime := stellspec.NewRuntime(typedCfg.Config)
	tracesConsumer, err := consumer.NewTraces(func(ctx context.Context, td ptrace.Traces) error {
		runtime.ProcessTraces(td)
		return next.ConsumeTraces(ctx, td)
	}, consumer.WithCapabilities(consumer.Capabilities{MutatesData: true}))
	if err != nil {
		return nil, err
	}

	return &tracesProcessor{
		StartFunc: func(context.Context, component.Host) error {
			set.Logger.Info("stellspec traces processor started", zap.String("processor", typeStr))
			return nil
		},
		ShutdownFunc: func(context.Context) error {
			set.Logger.Info("stellspec traces processor stopped", zap.String("processor", typeStr))
			return nil
		},
		Traces: tracesConsumer,
	}, nil
}
