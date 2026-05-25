package stellspec

import (
	"strings"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestNormalizeResourceFillsMissingValuesWithoutOverwritingExisting(t *testing.T) {
	t.Setenv("STELLAR_APP_NAMESPACE", "stellar.trade")

	cfg := NewDefaultConfig(SignalLogs)
	cfg.ResourceEnvMappings = []ResourceEnvMapping{
		{AttributeKey: "service.name", EnvVar: "STELLAR_APP_NAME"},
		{AttributeKey: "service.namespace", EnvVar: "STELLAR_APP_NAMESPACE"},
	}
	rt := NewRuntime(cfg)

	attrs := pcommon.NewMap()
	attrs.PutStr("service.name", "explicit-service")

	rt.NormalizeResource(attrs)

	assertStringAttr(t, attrs, "service.name", "explicit-service")
	assertStringAttr(t, attrs, "service.namespace", "stellar.trade")
	assertStringAttr(t, attrs, "stellspec.signal", SignalLogs)
}

func TestProcessLogRecordNormalizesSeverityRedactsBodyAndPreparesRouting(t *testing.T) {
	cfg := NewDefaultConfig(SignalLogs)
	cfg.MaxLogBodyBytes = 24
	rt := NewRuntime(cfg)

	record := plog.NewLogRecord()
	record.Attributes().PutStr("level", "warn")
	record.Body().SetStr("password=super-secret message")
	record.SetTraceID(pcommon.TraceID{1, 2, 3})

	rt.ProcessLogRecord(record)

	if record.SeverityText() != "WARN" {
		t.Fatalf("expected severity text WARN, got %q", record.SeverityText())
	}
	if record.SeverityNumber() != plog.SeverityNumberWarn {
		t.Fatalf("expected severity number WARN, got %s", record.SeverityNumber())
	}
	if body := record.Body().Str(); strings.Contains(body, "super-secret") || len(body) > cfg.MaxLogBodyBytes {
		t.Fatalf("expected redacted and truncated body, got %q", body)
	}
	assertStringAttr(t, record.Attributes(), "trace_id", record.TraceID().String())
	assertStringAttr(t, record.Attributes(), "stellspec.trace_id", record.TraceID().String())
	assertStringAttr(t, record.Attributes(), "stellspec.kafka.key", record.TraceID().String())
}

func TestProcessSpanNormalizesNameStatusAndTraceRouting(t *testing.T) {
	rt := NewRuntime(NewDefaultConfig(SignalTraces))

	span := ptrace.NewSpan()
	span.SetName("GET /users/123/orders/550e8400-e29b-41d4-a716-446655440000?debug=true")
	span.SetTraceID(pcommon.TraceID{9, 8, 7})
	span.Attributes().PutInt("http.response.status_code", 503)

	rt.ProcessSpan(span)

	if span.Name() != "GET /users/{id}/orders/{id}" {
		t.Fatalf("expected low-cardinality span name, got %q", span.Name())
	}
	if span.Status().Code() != ptrace.StatusCodeError {
		t.Fatalf("expected error status, got %s", span.Status().Code())
	}
	assertStringAttr(t, span.Attributes(), "error.type", "http.status_code.503")
	assertStringAttr(t, span.Attributes(), "stellspec.kafka.key", span.TraceID().String())
}

func TestProcessMetricNormalizesNameUnitTemporalityAndDropsHighCardinalityLabels(t *testing.T) {
	rt := NewRuntime(NewDefaultConfig(SignalMetrics))

	metric := pmetric.NewMetric()
	metric.SetName("HTTP Server Request Duration")
	metric.SetUnit("seconds")
	histogram := metric.SetEmptyHistogram()
	histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dataPoint := histogram.DataPoints().AppendEmpty()
	dataPoint.Attributes().PutStr("http.route", "/users/{id}")
	dataPoint.Attributes().PutStr("user_id", "42")

	rt.ProcessMetric(metric)

	if metric.Name() != "http_server_request_duration" {
		t.Fatalf("expected normalized metric name, got %q", metric.Name())
	}
	if metric.Unit() != "s" {
		t.Fatalf("expected normalized unit s, got %q", metric.Unit())
	}
	assertStringAttr(t, metric.Metadata(), "stellspec.aggregation_temporality", pmetric.AggregationTemporalityDelta.String())
	if _, ok := dataPoint.Attributes().Get("user_id"); ok {
		t.Fatal("expected high-cardinality user_id metric label to be removed")
	}
	assertStringAttr(t, dataPoint.Attributes(), "http.route", "/users/{id}")
}

func assertStringAttr(t *testing.T, attrs pcommon.Map, key, expected string) {
	t.Helper()
	value, ok := attrs.Get(key)
	if !ok {
		t.Fatalf("expected attribute %q", key)
	}
	if actual := value.Str(); actual != expected {
		t.Fatalf("expected attribute %q=%q, got %q", key, expected, actual)
	}
}
