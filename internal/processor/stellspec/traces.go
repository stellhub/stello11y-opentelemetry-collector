package stellspec

import (
	"regexp"
	"strings"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

var (
	uuidSegmentPattern      = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	longHexSegmentPattern   = regexp.MustCompile(`(?i)^[0-9a-f]{16,}$`)
	numericSegmentPattern   = regexp.MustCompile(`^[0-9]+$`)
	lowCardinalityPathToken = regexp.MustCompile(`\{[^/{}]+\}`)
)

func (rt *Runtime) ProcessTraces(td ptrace.Traces) {
	resourceSpans := td.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)
		rt.NormalizeResource(resourceSpan.Resource().Attributes())

		scopeSpans := resourceSpan.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			spans := scopeSpans.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				rt.ProcessSpan(spans.At(k))
			}
		}
	}
}

func (rt *Runtime) ProcessSpan(span ptrace.Span) {
	rt.SanitizeMap(span.Attributes())
	rt.normalizeSpanName(span)
	rt.normalizeSpanStatus(span)
	rt.prepareTraceRouting(span)

	events := span.Events()
	for i := 0; i < events.Len(); i++ {
		rt.SanitizeMap(events.At(i).Attributes())
	}

	links := span.Links()
	for i := 0; i < links.Len(); i++ {
		rt.SanitizeMap(links.At(i).Attributes())
	}
}

func (rt *Runtime) normalizeSpanName(span ptrace.Span) {
	attrs := span.Attributes()
	if route, ok := attrString(attrs, "http.route"); ok {
		if method, exists := attrString(attrs, "http.request.method", "http.method"); exists {
			span.SetName(truncateUTF8(strings.ToUpper(method)+" "+route, rt.cfg.MaxSpanNameBytes))
			return
		}
		span.SetName(truncateUTF8(route, rt.cfg.MaxSpanNameBytes))
		return
	}

	span.SetName(truncateUTF8(normalizeLowCardinalityName(span.Name()), rt.cfg.MaxSpanNameBytes))
}

func (rt *Runtime) normalizeSpanStatus(span ptrace.Span) {
	status := span.Status()
	if status.Code() != ptrace.StatusCodeUnset {
		return
	}

	attrs := span.Attributes()
	if statusCode, ok := attrInt(attrs, "http.response.status_code", "http.status_code"); ok && statusCode >= 500 {
		status.SetCode(ptrace.StatusCodeError)
		putStrIfMissing(attrs, "error.type", "http.status_code."+intString(statusCode))
		return
	}

	if statusCode, ok := attrInt(attrs, "rpc.grpc.status_code", "rpc.response.status_code"); ok && statusCode != 0 {
		status.SetCode(ptrace.StatusCodeError)
		putStrIfMissing(attrs, "error.type", "rpc.status_code."+intString(statusCode))
		return
	}

	if _, ok := attrString(attrs, "error.type"); ok {
		status.SetCode(ptrace.StatusCodeError)
	}
}

func (rt *Runtime) prepareTraceRouting(span ptrace.Span) {
	attrs := span.Attributes()
	putStrIfMissing(attrs, "stellspec.signal", SignalTraces)
	putStrIfMissing(attrs, "stellspec.kafka.topic", rt.cfg.KafkaTopic)
	putStrIfMissing(attrs, "stellspec.index.prefix", rt.cfg.IndexPrefix)

	if !span.TraceID().IsEmpty() {
		traceID := span.TraceID().String()
		putStrIfMissing(attrs, "stellspec.trace_id", traceID)
		putStrIfMissing(attrs, "stellspec.kafka.key", traceID)
	}
}

func normalizeLowCardinalityName(name string) string {
	name = strings.TrimSpace(strings.Split(name, "?")[0])
	if name == "" || !strings.Contains(name, "/") || lowCardinalityPathToken.MatchString(name) {
		return name
	}

	parts := strings.Split(name, "/")
	for i, part := range parts {
		if isDynamicPathSegment(part) {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

func isDynamicPathSegment(segment string) bool {
	segment = strings.TrimSpace(segment)
	return numericSegmentPattern.MatchString(segment) ||
		uuidSegmentPattern.MatchString(segment) ||
		longHexSegmentPattern.MatchString(segment)
}

func intString(value int64) string {
	if value == 0 {
		return "0"
	}

	negative := value < 0
	if negative {
		value = -value
	}

	buf := make([]byte, 0, 20)
	for value > 0 {
		buf = append(buf, byte('0'+value%10))
		value /= 10
	}
	if negative {
		buf = append(buf, '-')
	}

	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
