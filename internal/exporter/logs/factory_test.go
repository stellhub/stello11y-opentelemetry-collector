package logs

import (
	"encoding/json"
	"testing"

	"github.com/stellhub/stellflow-go-sdk/protocol/message"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestBuildRecordUsesConfiguredTopicAndTraceKey(t *testing.T) {
	le := &logsExporter{cfg: createDefaultConfig().(*Config)}
	resourceLog, scopeLog, record := newTestLogRecord()
	record.SetTraceID(pcommon.TraceID{1, 2, 3})
	record.Attributes().PutStr("stellspec.kafka.key", record.TraceID().String())

	producerRecord, err := le.buildRecord(resourceLog, scopeLog, record)
	if err != nil {
		t.Fatal(err)
	}

	if producerRecord.Topic != "stello11y.logs.app.prod.v1" {
		t.Fatalf("expected configured topic, got %q", producerRecord.Topic)
	}
	if string(producerRecord.Key) != record.TraceID().String() {
		t.Fatalf("expected trace partition key, got %q", string(producerRecord.Key))
	}
	assertHeader(t, producerRecord.Headers, "tenant.id", "tenant-a")
	assertHeader(t, producerRecord.Headers, "service.name", "orders")

	var envelope logEnvelope
	if err := json.Unmarshal(producerRecord.Value, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.TraceID != record.TraceID().String() {
		t.Fatalf("expected envelope trace id, got %q", envelope.TraceID)
	}
	if envelope.Body != "hello" {
		t.Fatalf("expected body hello, got %#v", envelope.Body)
	}
}

func TestResolveTopicWithEnvironmentSignalStrategy(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.TopicStrategy = topicStrategyEnvironmentSignal
	cfg.TopicPrefix = "stello11y.logs"
	cfg.LogCategory = "app"
	cfg.TopicVersion = "v1"
	le := &logsExporter{cfg: cfg}

	attrs := pcommon.NewMap()
	attrs.PutStr("deployment.environment.name", "Staging")

	topic := le.resolveTopic(attrs, pcommon.NewMap())
	if topic != "stello11y.logs.app.staging.v1" {
		t.Fatalf("expected environment topic, got %q", topic)
	}
}

func TestResolvePartitionKeyIgnoresLowValueFallbackKey(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	le := &logsExporter{cfg: cfg}
	_, _, record := newTestLogRecord()
	record.Attributes().PutStr("stellspec.kafka.key", "logs")

	resourceAttrs := pcommon.NewMap()
	resourceAttrs.PutStr("tenant.id", "tenant-a")
	resourceAttrs.PutStr("service.name", "orders")
	resourceAttrs.PutStr("service.instance.id", "instance-1")

	key := le.resolvePartitionKey(resourceAttrs, record)
	if key != "tenant-a/orders/instance-1" {
		t.Fatalf("expected service instance key, got %q", key)
	}
}

func newTestLogRecord() (plog.ResourceLogs, plog.ScopeLogs, plog.LogRecord) {
	ld := plog.NewLogs()
	resourceLog := ld.ResourceLogs().AppendEmpty()
	resourceLog.Resource().Attributes().PutStr("tenant.id", "tenant-a")
	resourceLog.Resource().Attributes().PutStr("service.name", "orders")
	resourceLog.Resource().Attributes().PutStr("service.namespace", "stellar.trade")
	resourceLog.Resource().Attributes().PutStr("deployment.environment.name", "prod")

	scopeLog := resourceLog.ScopeLogs().AppendEmpty()
	scopeLog.Scope().SetName("test-scope")

	record := scopeLog.LogRecords().AppendEmpty()
	record.Body().SetStr("hello")
	record.SetSeverityText("INFO")
	record.SetSeverityNumber(plog.SeverityNumberInfo)
	return resourceLog, scopeLog, record
}

func assertHeader(t *testing.T, headers []message.RecordHeader, key string, expected string) {
	t.Helper()
	for _, header := range headers {
		if header.Key != nil && *header.Key == key {
			if string(header.Value) != expected {
				t.Fatalf("expected header %q=%q, got %q", key, expected, string(header.Value))
			}
			return
		}
	}
	t.Fatalf("expected header %q", key)
}
