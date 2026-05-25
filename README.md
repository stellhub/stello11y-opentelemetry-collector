# stello11y-opentelemetry-collector

`stello11y-opentelemetry-collector` 是面向 `stellspec` 体系的自定义 OpenTelemetry Collector 发行版。

当前项目使用 OpenTelemetry 官方 OTLP Receiver 作为统一接入层，固定监听本机 `localhost:4317`，接收 `logs`、`metrics`、`traces` 三类信号，并在 Collector 内部完成平台标准化处理和后端导出。

## 架构

```text
Application / SDK / Framework
  -> OTLP/gRPC localhost:4317
  -> stello11y-opentelemetry-collector
  -> stellspec processors
  -> signal-specific exporters
  -> backend systems
```

当前信号出口如下：

| Signal | Export Target | Role |
| --- | --- | --- |
| Logs | Stellflow | 日志事件总线，后续由日志消费链路写入 Elasticsearch / OpenSearch / Loki |
| Metrics | VictoriaMetrics | Prometheus-compatible 时序数据库 |
| Traces | Grafana Tempo | 分布式链路追踪后端 |

## Receiver

当前只启用 OTLP/gRPC：

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: localhost:4317
```

## Processors

三类信号均经过自定义 `stellspec_*` processor。

### Resource 补齐

processor 只在 Resource 属性缺失时补齐字段，不覆盖上游已上报的非空标准字段。

补齐优先级：

1. OTLP Resource 中已有属性
2. `OTEL_RESOURCE_ATTRIBUTES`
3. `OTEL_SERVICE_NAME`
4. `STELLAR_*` 环境变量
5. processor `resource_attributes` 配置

内置 `STELLAR_*` 映射：

| Environment | Resource Attribute |
| --- | --- |
| `STELLAR_APP_NAME` | `service.name` |
| `STELLAR_APP_NAMESPACE` | `service.namespace` |
| `STELLAR_APP_VERSION` | `service.version` |
| `STELLAR_APP_INSTANCE_ID` | `service.instance.id` |
| `STELLAR_ENV` | `deployment.environment.name` |
| `STELLAR_CLUSTER` | `k8s.cluster.name` |
| `STELLAR_REGION` | `cloud.region` |
| `STELLAR_ZONE` | `cloud.availability_zone` |
| `STELLAR_HOST_NAME` | `host.name` |
| `STELLAR_HOST_IP` | `host.ip` |
| `STELLAR_NODE_NAME` | `k8s.node.name` |
| `STELLAR_K8S_NAMESPACE` | `k8s.namespace.name` |
| `STELLAR_POD_NAME` | `k8s.pod.name` |
| `STELLAR_POD_UID` | `k8s.pod.uid` |
| `STELLAR_POD_IP` | `k8s.pod.ip` |
| `STELLAR_CONTAINER_NAME` | `k8s.container.name` |

### Logs

`stellspec_logs` processor 执行：

- Resource 补齐
- `SeverityText` / `SeverityNumber` 标准化
- 日志属性和日志正文敏感字段脱敏
- 日志正文裁剪
- Trace 关联字段补齐
- Stellflow 路由提示字段补齐

有 trace 上下文的日志会在 LogRecord attributes 中补齐：

```text
trace_id
span_id
trace_sampled
stellspec.trace_id
stellspec.kafka.key
```

其中 `stellspec.kafka.key` 使用 `trace_id`，用于 Stellflow 分区路由。

### Metrics

`stellspec_metrics` processor 执行：

- Resource 补齐
- Metric name / unit 标准化
- 高基数 label 清理
- Metric metadata 标记
- 指标白名单 / 黑名单过滤

VictoriaMetrics 不接收 delta temporality 作为最终存储语义，因此 metrics pipeline 在 `stellspec_metrics` 后启用官方 `deltatocumulative` processor。

### Traces

`stellspec_traces` processor 执行：

- Resource 补齐
- Span attribute 清洗
- Span name 低基数化
- Span status 补齐
- Trace 路由字段补齐

## Exporters

### Logs To Stellflow

Logs 使用 `stellflow-go-sdk` Producer 写入 Stellflow。

默认 topic：

```text
stello11y.logs.app.prod.v1
```

默认不为每个应用创建独立 topic。应用、租户、环境和服务信息写入 record headers 与 JSON envelope。

默认 record headers：

```text
content-type=application/json
stello11y.signal=logs
tenant.id
service.name
service.namespace
deployment.environment.name
```

默认分区 key 策略：

```text
trace_or_service_instance
```

规则：

1. 有 `trace_id` 时，使用 `trace_id` 作为 key。
2. 无 `trace_id` 时，使用 `tenant_id/service.name/service.instance.id`。
3. 大流量应用可切换为 `trace_or_service_bucket`。

### Metrics To VictoriaMetrics

Metrics 使用官方 `otlphttp` exporter 直接写入 VictoriaMetrics。

单机 VictoriaMetrics endpoint：

```text
http://127.0.0.1:8428/opentelemetry/v1/metrics
```

VictoriaMetrics Cluster endpoint：

```text
http://<vminsert>:8480/insert/<accountID>/opentelemetry/v1/metrics
```

指标写入 VictoriaMetrics 后，Grafana 可使用 Prometheus datasource 或 VictoriaMetrics datasource 直接查询 VictoriaMetrics。Prometheus 不是这条链路的必需组件。

### Traces To Tempo

Traces 使用官方 `otlp` exporter 通过 OTLP/gRPC 写入 Grafana Tempo。

默认 Tempo endpoint：

```text
tempo:4317
```

本地或非 TLS 环境使用：

```yaml
otlp/tempo:
  endpoint: tempo:4317
  tls:
    insecure: true
```

`stellspec_traces` 只负责 Trace 数据治理，包括 Resource 补齐、Span 属性清洗、Span name 低基数化和 Span status 补齐；Trace 的传输格式保持 OpenTelemetry 官方 OTLP 标准。

## Configuration

当前默认配置：

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: localhost:4317

processors:
  stellspec_logs: {}
  stellspec_metrics: {}
  stellspec_traces: {}
  deltatocumulative:
    max_stale: 5m

exporters:
  stellspec_logs:
    backend: stellflow
    bootstrap_servers:
      - stellflow://127.0.0.1:9092
    client_id: stello11y-opentelemetry-collector
    topic: stello11y.logs.app.prod.v1
    topic_strategy: static
    partition_key_strategy: trace_or_service_instance

  otlphttp/victoriametrics:
    compression: gzip
    encoding: proto
    metrics_endpoint: http://127.0.0.1:8428/opentelemetry/v1/metrics

  otlp/tempo:
    endpoint: tempo:4317
    tls:
      insecure: true

service:
  pipelines:
    logs:
      receivers: [otlp]
      processors: [stellspec_logs]
      exporters: [stellspec_logs]

    metrics:
      receivers: [otlp]
      processors: [stellspec_metrics, deltatocumulative]
      exporters: [otlphttp/victoriametrics]

    traces:
      receivers: [otlp]
      processors: [stellspec_traces]
      exporters: [otlp/tempo]
```

## Project Layout

```text
stello11y-opentelemetry-collector/
├── cmd/
│   └── main.go
├── configs/
│   └── collector.yaml
├── docs/
│   └── collector-flow.svg
├── internal/
│   ├── collector/
│   ├── exporter/
│   ├── processor/
│   └── shared/
├── go.mod
└── README.md
```

## Validation

```powershell
go test ./...
go run .\cmd validate --config configs\collector.yaml
```

## License

Apache License 2.0
