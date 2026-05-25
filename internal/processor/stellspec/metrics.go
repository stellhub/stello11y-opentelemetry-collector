package stellspec

import (
	"strings"
	"unicode"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var metricHighCardinalityKeys = map[string]struct{}{
	"user_id":      {},
	"user.id":      {},
	"tenant_id":    {},
	"tenant.id":    {},
	"request_id":   {},
	"request.id":   {},
	"session_id":   {},
	"session.id":   {},
	"trace_id":     {},
	"trace.id":     {},
	"span_id":      {},
	"span.id":      {},
	"k8s.pod.name": {},
	"host.ip":      {},
	"url.full":     {},
	"http.target":  {},
	"http.url":     {},
	"url.query":    {},
	"client.ip":    {},
	"enduser.id":   {},
	"device.id":    {},
}

var stableMetricKeys = map[string]struct{}{
	"http.request.method":       {},
	"http.response.status_code": {},
	"http.route":                {},
	"rpc.system.name":           {},
	"rpc.method":                {},
	"rpc.response.status_code":  {},
	"server.address":            {},
	"server.port":               {},
	"url.scheme":                {},
	"network.protocol.name":     {},
	"network.protocol.version":  {},
	"error.type":                {},
}

func (rt *Runtime) ProcessMetrics(md pmetric.Metrics) {
	resourceMetrics := md.ResourceMetrics()
	for i := 0; i < resourceMetrics.Len(); i++ {
		resourceMetric := resourceMetrics.At(i)
		rt.NormalizeResource(resourceMetric.Resource().Attributes())

		scopeMetrics := resourceMetric.ScopeMetrics()
		for j := 0; j < scopeMetrics.Len(); j++ {
			metrics := scopeMetrics.At(j).Metrics()
			metrics.RemoveIf(func(metric pmetric.Metric) bool {
				return rt.shouldDropMetric(metric.Name())
			})
			for k := 0; k < metrics.Len(); k++ {
				rt.ProcessMetric(metrics.At(k))
			}
		}
	}
}

func (rt *Runtime) ProcessMetric(metric pmetric.Metric) {
	metric.SetName(normalizeMetricName(metric.Name()))
	metric.SetUnit(normalizeMetricUnit(metric.Name(), metric.Unit()))
	rt.markMetricTemporality(metric)
	rt.processMetricDataPointAttributes(metric)
}

func (rt *Runtime) shouldDropMetric(name string) bool {
	name = strings.TrimSpace(name)
	if len(rt.includeMetrics) > 0 {
		if _, ok := rt.includeMetrics[name]; !ok {
			return true
		}
	}
	if _, ok := rt.excludeMetrics[name]; ok {
		return true
	}
	return false
}

func normalizeMetricName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}

	var builder strings.Builder
	builder.Grow(len(name))
	lastSeparator := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-':
			builder.WriteRune(unicode.ToLower(r))
			lastSeparator = false
		default:
			if !lastSeparator {
				builder.WriteByte('_')
				lastSeparator = true
			}
		}
	}
	return strings.Trim(builder.String(), "_")
}

func normalizeMetricUnit(name, unit string) string {
	unit = strings.TrimSpace(unit)
	switch strings.ToLower(unit) {
	case "second", "seconds", "sec", "secs":
		return "s"
	case "millisecond", "milliseconds", "msec", "msecs":
		return "ms"
	case "byte", "bytes":
		return "By"
	case "request", "requests":
		return "{request}"
	}

	if unit == "" {
		switch {
		case strings.HasSuffix(name, ".duration"):
			return "s"
		case strings.HasSuffix(name, ".active_requests"):
			return "{request}"
		}
	}
	return unit
}

func (rt *Runtime) markMetricTemporality(metric pmetric.Metric) {
	metadata := metric.Metadata()
	putStrIfMissing(metadata, "stellspec.signal", SignalMetrics)
	putStrIfMissing(metadata, "stellspec.kafka.topic", rt.cfg.KafkaTopic)
	putStrIfMissing(metadata, "stellspec.index.prefix", rt.cfg.IndexPrefix)
	putStrIfMissing(metadata, "stellspec.metric.type", metric.Type().String())

	switch metric.Type() {
	case pmetric.MetricTypeSum:
		putStrIfMissing(metadata, "stellspec.aggregation_temporality", metric.Sum().AggregationTemporality().String())
	case pmetric.MetricTypeHistogram:
		putStrIfMissing(metadata, "stellspec.aggregation_temporality", metric.Histogram().AggregationTemporality().String())
	case pmetric.MetricTypeExponentialHistogram:
		putStrIfMissing(metadata, "stellspec.aggregation_temporality", metric.ExponentialHistogram().AggregationTemporality().String())
	}
}

func (rt *Runtime) processMetricDataPointAttributes(metric pmetric.Metric) {
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		dataPoints := metric.Gauge().DataPoints()
		for i := 0; i < dataPoints.Len(); i++ {
			rt.processMetricAttrs(dataPoints.At(i).Attributes())
		}
	case pmetric.MetricTypeSum:
		dataPoints := metric.Sum().DataPoints()
		for i := 0; i < dataPoints.Len(); i++ {
			rt.processMetricAttrs(dataPoints.At(i).Attributes())
		}
	case pmetric.MetricTypeHistogram:
		dataPoints := metric.Histogram().DataPoints()
		for i := 0; i < dataPoints.Len(); i++ {
			rt.processMetricAttrs(dataPoints.At(i).Attributes())
		}
	case pmetric.MetricTypeExponentialHistogram:
		dataPoints := metric.ExponentialHistogram().DataPoints()
		for i := 0; i < dataPoints.Len(); i++ {
			rt.processMetricAttrs(dataPoints.At(i).Attributes())
		}
	case pmetric.MetricTypeSummary:
		dataPoints := metric.Summary().DataPoints()
		for i := 0; i < dataPoints.Len(); i++ {
			rt.processMetricAttrs(dataPoints.At(i).Attributes())
		}
	}
}

func (rt *Runtime) processMetricAttrs(attrs pcommon.Map) {
	rt.SanitizeMap(attrs)
	attrs.RemoveIf(func(key string, _ pcommon.Value) bool {
		_, drop := metricHighCardinalityKeys[strings.ToLower(key)]
		return drop
	})

	if rt.cfg.MaxMetricAttributes <= 0 {
		return
	}
	for attrs.Len() > rt.cfg.MaxMetricAttributes {
		removed := false
		attrs.Range(func(key string, _ pcommon.Value) bool {
			if _, stable := stableMetricKeys[key]; stable {
				return true
			}
			attrs.Remove(key)
			removed = true
			return false
		})
		if !removed {
			break
		}
	}
}
