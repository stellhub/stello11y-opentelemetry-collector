package logs

import (
	"bytes"
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
	record.SetSpanID(pcommon.SpanID{4, 5, 6})
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
	if envelope.SpanID != record.SpanID().String() {
		t.Fatalf("expected envelope span id, got %q", envelope.SpanID)
	}
	if envelope.Body != "hello" {
		t.Fatalf("expected body hello, got %#v", envelope.Body)
	}
}

func TestBuildRecordAlignsEnvelopeWithStellspecConsumer(t *testing.T) {
	le := &logsExporter{cfg: createDefaultConfig().(*Config)}
	resourceLog, scopeLog, record := newTestLogRecord()
	record.SetTimestamp(pcommon.Timestamp(1_700_000_000_000_000_001))
	record.SetObservedTimestamp(pcommon.Timestamp(1_700_000_000_000_000_002))
	record.SetTraceID(pcommon.TraceID{1, 2, 3})
	record.SetSpanID(pcommon.SpanID{4, 5, 6})

	producerRecord, err := le.buildRecord(resourceLog, scopeLog, record)
	if err != nil {
		t.Fatal(err)
	}

	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(producerRecord.Value))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		t.Fatal(err)
	}

	assertJSONNumber(t, payload, "timeUnixNano", 1_700_000_000_000_000_001)
	assertJSONNumber(t, payload, "observedTimeUnixNano", 1_700_000_000_000_000_002)
	assertJSONValue(t, payload, "severityText", "INFO")
	assertJSONValue(t, payload, "severityNumber", plog.SeverityNumberInfo.String())
	assertJSONValue(t, payload, "traceId", record.TraceID().String())
	assertJSONValue(t, payload, "spanId", record.SpanID().String())

	assertJSONNumber(t, payload, "timestamp_unix_nano", 1_700_000_000_000_000_001)
	assertJSONValue(t, payload, "severity_text", "INFO")
	assertJSONValue(t, payload, "trace_id", record.TraceID().String())

	attributes := assertObject(t, payload, "attributes")
	assertJSONValue(t, attributes, "tenant.id", "tenant-a")
	if eventID := stringValue(attributes["stellspec.event_id"]); eventID == "" {
		t.Fatalf("expected stellspec.event_id in payload attributes")
	}

	resource := assertObject(t, payload, "resource")
	assertJSONValue(t, resource, "service.name", "orders")
	assertJSONValue(t, resource, "deployment.environment.name", "prod")
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

func assertJSONValue(t *testing.T, values map[string]any, key string, expected string) {
	t.Helper()
	if actual := stringValue(values[key]); actual != expected {
		t.Fatalf("expected json %q=%q, got %#v", key, expected, values[key])
	}
}

func assertJSONNumber(t *testing.T, values map[string]any, key string, expected uint64) {
	t.Helper()
	actual, ok := values[key].(json.Number)
	if !ok {
		t.Fatalf("expected json %q to be a number, got %#v", key, values[key])
	}
	parsed, err := actual.Int64()
	if err != nil {
		t.Fatalf("expected json %q to parse as int64: %v", key, err)
	}
	if uint64(parsed) != expected {
		t.Fatalf("expected json %q=%d, got %s", key, expected, actual)
	}
}

func assertObject(t *testing.T, values map[string]any, key string) map[string]any {
	t.Helper()
	object, ok := values[key].(map[string]any)
	if !ok {
		t.Fatalf("expected json %q to be an object, got %#v", key, values[key])
	}
	return object
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
