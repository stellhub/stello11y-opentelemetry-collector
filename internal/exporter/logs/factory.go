package logs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stellhub/stellflow-go-sdk/producer"
	"github.com/stellhub/stellflow-go-sdk/protocol/message"
	"github.com/stellhub/stellflow-go-sdk/stellflow"
	"github.com/stellhub/stello11y-opentelemetry-collector/internal/shared/datacount"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

const (
	typeStr = "stellspec_logs"

	backendStellflow = "stellflow"
	backendNoop      = "noop"

	topicStrategyStatic            = "static"
	topicStrategyEnvironmentSignal = "environment_signal"

	partitionKeyTraceOrServiceInstance = "trace_or_service_instance"
	partitionKeyTraceOrServiceBucket   = "trace_or_service_bucket"
)

var typeID = component.MustNewType(typeStr)

type Config struct {
	Backend              string        `mapstructure:"backend"`
	BootstrapServers     []string      `mapstructure:"bootstrap_servers"`
	ClientID             string        `mapstructure:"client_id"`
	Topic                string        `mapstructure:"topic"`
	TopicStrategy        string        `mapstructure:"topic_strategy"`
	TopicPrefix          string        `mapstructure:"topic_prefix"`
	TopicVersion         string        `mapstructure:"topic_version"`
	LogCategory          string        `mapstructure:"log_category"`
	DefaultTenantID      string        `mapstructure:"default_tenant_id"`
	PartitionKeyStrategy string        `mapstructure:"partition_key_strategy"`
	SendTimeout          time.Duration `mapstructure:"send_timeout"`
	Acks                 int16         `mapstructure:"acks"`
	TimeoutMs            int32         `mapstructure:"timeout_ms"`
	BatchSize            int           `mapstructure:"batch_size"`
	BatchBytes           int           `mapstructure:"batch_bytes"`
	Linger               time.Duration `mapstructure:"linger"`
	QueueSize            int           `mapstructure:"queue_size"`
}

type logsExporter struct {
	set      exporter.Settings
	cfg      *Config
	factory  *stellflow.ClientFactory
	producer *producer.Client
}

type logEnvelope struct {
	Signal                    string         `json:"signal"`
	TimestampUnixNano         uint64         `json:"timestamp_unix_nano,omitempty"`
	ObservedTimestampUnixNano uint64         `json:"observed_timestamp_unix_nano,omitempty"`
	SeverityText              string         `json:"severity_text,omitempty"`
	SeverityNumber            string         `json:"severity_number,omitempty"`
	TraceID                   string         `json:"trace_id,omitempty"`
	SpanID                    string         `json:"span_id,omitempty"`
	TraceSampled              bool           `json:"trace_sampled,omitempty"`
	Body                      any            `json:"body,omitempty"`
	Attributes                map[string]any `json:"attributes,omitempty"`
	Resource                  map[string]any `json:"resource,omitempty"`
	ResourceSchemaURL         string         `json:"resource_schema_url,omitempty"`
	Scope                     scopeEnvelope  `json:"scope,omitempty"`
	ScopeSchemaURL            string         `json:"scope_schema_url,omitempty"`
}

type scopeEnvelope struct {
	Name       string         `json:"name,omitempty"`
	Version    string         `json:"version,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		typeID,
		createDefaultConfig,
		exporter.WithLogs(createLogsExporter, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Backend:              backendStellflow,
		BootstrapServers:     []string{"stellflow://127.0.0.1:9092"},
		ClientID:             "stello11y-opentelemetry-collector",
		Topic:                "stello11y.logs.app.prod.v1",
		TopicStrategy:        topicStrategyStatic,
		TopicPrefix:          "stello11y.logs",
		TopicVersion:         "v1",
		LogCategory:          "app",
		DefaultTenantID:      "default",
		PartitionKeyStrategy: partitionKeyTraceOrServiceInstance,
		SendTimeout:          30 * time.Second,
		Acks:                 -1,
		TimeoutMs:            30000,
		BatchSize:            100,
		BatchBytes:           1024 * 1024,
		Linger:               5 * time.Millisecond,
		QueueSize:            1024,
	}
}

func createLogsExporter(ctx context.Context, set exporter.Settings, cfg component.Config) (exporter.Logs, error) {
	typedCfg := cfg.(*Config)
	normalizeConfig(typedCfg)
	le := &logsExporter{set: set, cfg: typedCfg}

	return exporterhelper.NewLogs(
		ctx,
		set,
		cfg,
		le.consumeLogs,
		exporterhelper.WithStart(le.start),
		exporterhelper.WithShutdown(le.shutdown),
	)
}

func normalizeConfig(cfg *Config) {
	cfg.Backend = strings.TrimSpace(strings.ToLower(cfg.Backend))
	if cfg.Backend == "" {
		cfg.Backend = backendStellflow
	}
	if cfg.ClientID == "" {
		cfg.ClientID = "stello11y-opentelemetry-collector"
	}
	if cfg.Topic == "" {
		cfg.Topic = "stello11y.logs.app.prod.v1"
	}
	if cfg.TopicStrategy == "" {
		cfg.TopicStrategy = topicStrategyStatic
	}
	if cfg.TopicPrefix == "" {
		cfg.TopicPrefix = "stello11y.logs"
	}
	if cfg.TopicVersion == "" {
		cfg.TopicVersion = "v1"
	}
	if cfg.LogCategory == "" {
		cfg.LogCategory = "app"
	}
	if cfg.DefaultTenantID == "" {
		cfg.DefaultTenantID = "default"
	}
	if cfg.PartitionKeyStrategy == "" {
		cfg.PartitionKeyStrategy = partitionKeyTraceOrServiceInstance
	}
	if cfg.SendTimeout <= 0 {
		cfg.SendTimeout = 30 * time.Second
	}
	if cfg.Acks == 0 {
		cfg.Acks = -1
	}
	if cfg.TimeoutMs == 0 {
		cfg.TimeoutMs = 30000
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.BatchBytes <= 0 {
		cfg.BatchBytes = 1024 * 1024
	}
	if cfg.Linger <= 0 {
		cfg.Linger = 5 * time.Millisecond
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
}

func (le *logsExporter) start(context.Context, component.Host) error {
	if le.cfg.Backend == backendNoop {
		le.set.Logger.Info("stellspec logs exporter started in noop mode", zap.String("exporter", typeStr))
		return nil
	}
	if le.cfg.Backend != backendStellflow {
		return fmt.Errorf("unsupported logs backend %q", le.cfg.Backend)
	}
	if len(le.cfg.BootstrapServers) == 0 {
		return errors.New("bootstrap_servers must not be empty when backend is stellflow")
	}

	factory, err := stellflow.NewClientFactory(stellflow.Options{
		BootstrapServers: le.cfg.BootstrapServers,
		ClientID:         le.cfg.ClientID,
		Producer: producer.Options{
			Acks:       le.cfg.Acks,
			TimeoutMs:  le.cfg.TimeoutMs,
			BatchSize:  le.cfg.BatchSize,
			BatchBytes: le.cfg.BatchBytes,
			Linger:     le.cfg.Linger,
			QueueSize:  le.cfg.QueueSize,
		},
	})
	if err != nil {
		return err
	}

	le.factory = factory
	le.producer = factory.NewProducer()
	le.set.Logger.Info(
		"stellspec logs exporter started",
		zap.String("exporter", typeStr),
		zap.String("backend", le.cfg.Backend),
		zap.Strings("bootstrap_servers", le.cfg.BootstrapServers),
		zap.String("topic", le.cfg.Topic),
		zap.String("topic_strategy", le.cfg.TopicStrategy),
		zap.String("partition_key_strategy", le.cfg.PartitionKeyStrategy),
	)
	return nil
}

func (le *logsExporter) shutdown(ctx context.Context) error {
	if le.producer != nil {
		if err := le.producer.Close(ctx); err != nil {
			return err
		}
	}
	if le.factory != nil {
		if err := le.factory.Close(); err != nil {
			return err
		}
	}
	le.set.Logger.Info("stellspec logs exporter stopped", zap.String("exporter", typeStr), zap.String("backend", le.cfg.Backend))
	return nil
}

func (le *logsExporter) consumeLogs(ctx context.Context, ld plog.Logs) error {
	if le.cfg.Backend == backendNoop {
		le.set.Logger.Info(
			"stellspec logs exporter consumed batch",
			zap.String("exporter", typeStr),
			zap.String("backend", le.cfg.Backend),
			zap.Int("resource_logs", ld.ResourceLogs().Len()),
			zap.Int("log_records", datacount.LogRecords(ld)),
		)
		return nil
	}
	if le.producer == nil {
		return errors.New("stellflow producer is not started")
	}

	sendCtx, cancel := context.WithTimeout(ctx, le.cfg.SendTimeout)
	defer cancel()

	futures := make([]*producer.Future, 0, datacount.LogRecords(ld))
	resourceLogs := ld.ResourceLogs()
	for i := 0; i < resourceLogs.Len(); i++ {
		resourceLog := resourceLogs.At(i)

		scopeLogs := resourceLog.ScopeLogs()
		for j := 0; j < scopeLogs.Len(); j++ {
			scopeLog := scopeLogs.At(j)
			records := scopeLog.LogRecords()
			for k := 0; k < records.Len(); k++ {
				record, err := le.buildRecord(resourceLog, scopeLog, records.At(k))
				if err != nil {
					return err
				}
				future, err := le.producer.SendAsync(sendCtx, record)
				if err != nil {
					return err
				}
				futures = append(futures, future)
			}
		}
	}

	if err := le.producer.Flush(sendCtx); err != nil {
		return err
	}
	for _, future := range futures {
		if _, err := future.Await(sendCtx); err != nil {
			return err
		}
	}

	le.set.Logger.Info(
		"stellspec logs exporter sent batch to stellflow",
		zap.String("exporter", typeStr),
		zap.String("backend", le.cfg.Backend),
		zap.Int("resource_logs", ld.ResourceLogs().Len()),
		zap.Int("log_records", len(futures)),
	)
	return nil
}

func (le *logsExporter) buildRecord(resourceLog plog.ResourceLogs, scopeLog plog.ScopeLogs, record plog.LogRecord) (producer.Record, error) {
	topic := le.resolveTopic(resourceLog.Resource().Attributes(), record.Attributes())
	key := le.resolvePartitionKey(resourceLog.Resource().Attributes(), record)
	envelope := buildEnvelope(resourceLog, scopeLog, record)
	value, err := json.Marshal(envelope)
	if err != nil {
		return producer.Record{}, err
	}

	return producer.Record{
		Topic: topic,
		Key:   []byte(key),
		Value: value,
		Headers: []message.RecordHeader{
			header("content-type", "application/json"),
			header("stello11y.signal", "logs"),
			header("tenant.id", le.resolveTenantID(resourceLog.Resource().Attributes(), record.Attributes())),
			header("service.name", attrString(resourceLog.Resource().Attributes(), "service.name")),
			header("service.namespace", attrString(resourceLog.Resource().Attributes(), "service.namespace")),
			header("deployment.environment.name", attrString(resourceLog.Resource().Attributes(), "deployment.environment.name")),
		},
	}, nil
}

func buildEnvelope(resourceLog plog.ResourceLogs, scopeLog plog.ScopeLogs, record plog.LogRecord) logEnvelope {
	scope := scopeLog.Scope()
	envelope := logEnvelope{
		Signal:                    "logs",
		TimestampUnixNano:         uint64(record.Timestamp()),
		ObservedTimestampUnixNano: uint64(record.ObservedTimestamp()),
		SeverityText:              record.SeverityText(),
		SeverityNumber:            record.SeverityNumber().String(),
		TraceSampled:              record.Flags().IsSampled(),
		Body:                      record.Body().AsRaw(),
		Attributes:                record.Attributes().AsRaw(),
		Resource:                  resourceLog.Resource().Attributes().AsRaw(),
		ResourceSchemaURL:         resourceLog.SchemaUrl(),
		Scope: scopeEnvelope{
			Name:       scope.Name(),
			Version:    scope.Version(),
			Attributes: scope.Attributes().AsRaw(),
		},
		ScopeSchemaURL: scopeLog.SchemaUrl(),
	}
	if !record.TraceID().IsEmpty() {
		envelope.TraceID = record.TraceID().String()
	}
	if !record.SpanID().IsEmpty() {
		envelope.SpanID = record.SpanID().String()
	}
	return envelope
}

func (le *logsExporter) resolveTopic(resourceAttrs pcommon.Map, logAttrs pcommon.Map) string {
	if le.cfg.TopicStrategy == topicStrategyEnvironmentSignal {
		env := attrString(resourceAttrs, "deployment.environment.name")
		if env == "" {
			env = "prod"
		}
		return strings.Join([]string{le.cfg.TopicPrefix, le.cfg.LogCategory, sanitizeTopicPart(env), le.cfg.TopicVersion}, ".")
	}
	return le.cfg.Topic
}

func (le *logsExporter) resolvePartitionKey(resourceAttrs pcommon.Map, record plog.LogRecord) string {
	if key := attrString(record.Attributes(), "stellspec.kafka.key"); isUsablePartitionKey(key) {
		return key
	}
	if !record.TraceID().IsEmpty() {
		return record.TraceID().String()
	}

	tenantID := le.resolveTenantID(resourceAttrs, record.Attributes())
	serviceName := attrString(resourceAttrs, "service.name")
	if serviceName == "" {
		serviceName = "unknown_service"
	}

	switch le.cfg.PartitionKeyStrategy {
	case partitionKeyTraceOrServiceBucket:
		bucket := stableBucket(record, 16)
		return strings.Join([]string{tenantID, serviceName, bucket}, "/")
	default:
		instanceID := attrString(resourceAttrs, "service.instance.id")
		if instanceID == "" {
			instanceID = attrString(resourceAttrs, "k8s.pod.name")
		}
		if instanceID == "" {
			instanceID = "unknown_instance"
		}
		return strings.Join([]string{tenantID, serviceName, instanceID}, "/")
	}
}

func isUsablePartitionKey(key string) bool {
	key = strings.TrimSpace(key)
	return key != "" && key != "logs"
}

func (le *logsExporter) resolveTenantID(resourceAttrs pcommon.Map, logAttrs pcommon.Map) string {
	for _, attrs := range []pcommon.Map{logAttrs, resourceAttrs} {
		if tenantID := attrString(attrs, "tenant.id", "tenant_id", "stello11y.tenant_id", "x-stellar-tenant-id", "X-Stellar-Tenant-Id"); tenantID != "" {
			return tenantID
		}
	}
	return le.cfg.DefaultTenantID
}

func attrString(attrs pcommon.Map, keys ...string) string {
	for _, key := range keys {
		if value, ok := attrs.Get(key); ok {
			if s := strings.TrimSpace(value.AsString()); s != "" {
				return s
			}
		}
	}
	return ""
}

func header(key, value string) message.RecordHeader {
	keyCopy := key
	return message.RecordHeader{Key: &keyCopy, Value: []byte(value)}
}

func sanitizeTopicPart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "unknown"
	}

	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func stableBucket(record plog.LogRecord, buckets int) string {
	if buckets <= 0 {
		buckets = 16
	}
	seed := record.Timestamp()
	if seed == 0 {
		seed = record.ObservedTimestamp()
	}
	return fmt.Sprintf("bucket-%02d", int(uint64(seed)%uint64(buckets)))
}
