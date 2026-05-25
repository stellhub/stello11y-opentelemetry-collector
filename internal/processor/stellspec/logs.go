package stellspec

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func (rt *Runtime) ProcessLogs(ld plog.Logs) {
	resourceLogs := ld.ResourceLogs()
	for i := 0; i < resourceLogs.Len(); i++ {
		resourceLog := resourceLogs.At(i)
		rt.NormalizeResource(resourceLog.Resource().Attributes())

		scopeLogs := resourceLog.ScopeLogs()
		for j := 0; j < scopeLogs.Len(); j++ {
			logRecords := scopeLogs.At(j).LogRecords()
			for k := 0; k < logRecords.Len(); k++ {
				rt.ProcessLogRecord(logRecords.At(k))
			}
		}
	}
}

func (rt *Runtime) ProcessLogRecord(record plog.LogRecord) {
	rt.normalizeSeverity(record)
	rt.SanitizeMap(record.Attributes())
	rt.sanitizeLogBody(record)
	rt.prepareLogRouting(record)
}

func (rt *Runtime) normalizeSeverity(record plog.LogRecord) {
	text := strings.TrimSpace(record.SeverityText())
	if text == "" {
		if level, ok := attrString(record.Attributes(), "level", "log.level", "severity"); ok {
			text = level
		}
	}

	if text != "" {
		normalizedText, number := normalizeSeverityText(text)
		record.SetSeverityText(normalizedText)
		if record.SeverityNumber() == plog.SeverityNumberUnspecified {
			record.SetSeverityNumber(number)
		}
		return
	}

	if record.SeverityNumber() != plog.SeverityNumberUnspecified {
		record.SetSeverityText(severityText(record.SeverityNumber()))
	}
}

func (rt *Runtime) sanitizeLogBody(record plog.LogRecord) {
	body := record.Body()
	rt.SanitizeValue(body)
	if body.Type() != pcommon.ValueTypeStr {
		return
	}
	original := body.Str()
	truncated := truncateUTF8(original, rt.cfg.MaxLogBodyBytes)
	body.SetStr(truncated)
	if len(original) > len(truncated) {
		record.Attributes().PutBool("stellspec.log.body_truncated", true)
	}
}

func (rt *Runtime) prepareLogRouting(record plog.LogRecord) {
	attrs := record.Attributes()
	putStrIfMissing(attrs, "stellspec.signal", SignalLogs)
	putStrIfMissing(attrs, "stellspec.kafka.topic", rt.cfg.KafkaTopic)
	putStrIfMissing(attrs, "stellspec.index.prefix", rt.cfg.IndexPrefix)

	if !record.TraceID().IsEmpty() {
		traceID := record.TraceID().String()
		putStrIfMissing(attrs, "trace_id", traceID)
		putStrIfMissing(attrs, "stellspec.trace_id", traceID)
		putStrIfMissing(attrs, "stellspec.kafka.key", traceID)
	}
	if !record.SpanID().IsEmpty() {
		putStrIfMissing(attrs, "span_id", record.SpanID().String())
	}
	if record.Flags().IsSampled() {
		attrs.PutBool("trace_sampled", true)
	}
	if !record.TraceID().IsEmpty() {
		return
	}

	putStrIfMissing(attrs, "stellspec.kafka.key", SignalLogs)
}

func normalizeSeverityText(value string) (string, plog.SeverityNumber) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "trace", "trc":
		return "TRACE", plog.SeverityNumberTrace
	case "debug", "dbg":
		return "DEBUG", plog.SeverityNumberDebug
	case "info", "information", "notice":
		return "INFO", plog.SeverityNumberInfo
	case "warn", "warning":
		return "WARN", plog.SeverityNumberWarn
	case "error", "err":
		return "ERROR", plog.SeverityNumberError
	case "fatal", "panic", "critical":
		return "FATAL", plog.SeverityNumberFatal
	default:
		return strings.ToUpper(strings.TrimSpace(value)), plog.SeverityNumberUnspecified
	}
}

func severityText(number plog.SeverityNumber) string {
	switch {
	case number >= plog.SeverityNumberFatal:
		return "FATAL"
	case number >= plog.SeverityNumberError:
		return "ERROR"
	case number >= plog.SeverityNumberWarn:
		return "WARN"
	case number >= plog.SeverityNumberInfo:
		return "INFO"
	case number >= plog.SeverityNumberDebug:
		return "DEBUG"
	case number >= plog.SeverityNumberTrace:
		return "TRACE"
	default:
		return ""
	}
}
