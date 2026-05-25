package datacount

import (
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func LogRecords(ld plog.Logs) int {
	total := 0
	resourceLogs := ld.ResourceLogs()
	for i := 0; i < resourceLogs.Len(); i++ {
		scopeLogs := resourceLogs.At(i).ScopeLogs()
		for j := 0; j < scopeLogs.Len(); j++ {
			total += scopeLogs.At(j).LogRecords().Len()
		}
	}
	return total
}

func MetricDataPoints(md pmetric.Metrics) int {
	total := 0
	resourceMetrics := md.ResourceMetrics()
	for i := 0; i < resourceMetrics.Len(); i++ {
		scopeMetrics := resourceMetrics.At(i).ScopeMetrics()
		for j := 0; j < scopeMetrics.Len(); j++ {
			metrics := scopeMetrics.At(j).Metrics()
			for k := 0; k < metrics.Len(); k++ {
				metric := metrics.At(k)
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					total += metric.Gauge().DataPoints().Len()
				case pmetric.MetricTypeSum:
					total += metric.Sum().DataPoints().Len()
				case pmetric.MetricTypeHistogram:
					total += metric.Histogram().DataPoints().Len()
				case pmetric.MetricTypeExponentialHistogram:
					total += metric.ExponentialHistogram().DataPoints().Len()
				case pmetric.MetricTypeSummary:
					total += metric.Summary().DataPoints().Len()
				}
			}
		}
	}
	return total
}

func Spans(td ptrace.Traces) int {
	total := 0
	resourceSpans := td.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		scopeSpans := resourceSpans.At(i).ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			total += scopeSpans.At(j).Spans().Len()
		}
	}
	return total
}
