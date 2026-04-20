# spectrum-log-agent

`spectrum-log-agent` 是 `Spectrum · 星谱` 在业务主机、虚拟机或 Kubernetes 节点侧的本机日志代理，负责接收 `spectrum-go-sdk` 通过 OTLP 协议发送的 `LogRecord`，在本机完成标准化、增强、清洗、缓冲、重试与路由后，再以 Kafka Producer 协议写入 Kafka。

当前项目的核心定位不是让业务应用直接耦合 Kafka，而是在应用进程与日志平台之间建立一个稳定的本机协议边界：

```text
业务应用
  -> spectrum-go-sdk
  -> OTLP/gRPC
  -> spectrum-log-agent
  -> Kafka
  -> Spectrum · 星谱后端链路
```

## 项目定位

根据 `Stellar Axis（星轴）` 与 `Spectrum · 星谱` 体系的职责划分：

- `spectrum-go-sdk`
  负责在业务进程内桥接 `zap / slog`，把日志转换为 OpenTelemetry `LogRecord`
- `spectrum-log-agent`
  负责作为本机统一日志入口，接收 OTLP/gRPC 日志请求，并完成本机处理与可靠转发
- Kafka 以及后端日志平台
  负责日志总线、存储、索引、检索、分析与告警

也就是说，业务应用只面向本机 `log agent`，不直接感知 Kafka broker、Topic 拓扑、认证细节和后端日志平台内部实现。

## 设计目标

`spectrum-log-agent` 的建设目标如下：

- 基于 OTLP/gRPC 接收来自 `spectrum-go-sdk` 的日志请求
- 兼容 OpenTelemetry `LogRecord` 语义，作为日志采集链路的稳定协议边界
- 在本机完成标准日志处理、字段清洗、上下文增强和路由决策
- 以高吞吐、低延迟、批量化、异步化方式写入 Kafka
- 在 Kafka 抖动、短暂不可用或下游限流时尽量不丢日志
- 明确提供降级、补偿、回放和观测能力，而不是静默丢弃
- 保持与 `Stellar Axis（星轴）` 的全局环境变量规范、请求头规范和指标规范一致

## 推荐职责边界

### `log agent` 应该负责的事情

- 接收 OTLP/gRPC 日志请求
- 对 `LogRecord` 做协议校验和字段标准化
- 把 `STELLAR_*` 基础元数据和请求上下文映射为统一日志字段
- 清洗异常字段、超长字段、非法字符和敏感数据
- 执行批量、重试、背压、熔断和本地缓冲
- 将日志转为统一 Kafka 消息结构并发送到 Kafka
- 暴露健康检查、内部指标和故障告警信号
- 在进程退出或重启时尽可能保证缓冲日志可恢复

### `log agent` 不应该负责的事情

- 改变业务应用原始日志 API 的使用方式
- 让业务应用直接感知 Kafka Producer 细节
- 在本机做复杂检索、索引和长期存储
- 承担业务级日志语义解释或业务规则编排

## 总体链路

推荐链路如下：

```text
业务应用进程内:
  zap / slog
    -> spectrum-go-sdk
    -> OpenTelemetry LogRecord
    -> OTLP exporter
    -> localhost:4317

本机 log agent 进程内:
  OTLP gRPC receiver
    -> protocol validation
    -> normalization
    -> enrich / sanitize
    -> memory queue
    -> batcher
    -> retry / backoff
    -> local disk spool (optional but strongly recommended)
    -> Kafka async producer

后端:
  Kafka
    -> downstream consumer / storage / indexing / alerting
```

推荐 `log agent` 的默认监听地址：

```text
localhost:4317
```

这与 `spectrum-go-sdk` README 中约定的本机 OTLP/gRPC 发送目标保持一致。

## 标准日志处理职责

一个高可用、高性能的 `log agent` 不应该只做“收了就转发”，而应该在本机完成以下标准处理。

### 1. 协议接入与请求校验

- 接收符合 OTLP 协议的 gRPC 请求
- 校验 `ResourceLogs / ScopeLogs / LogRecords` 结构完整性
- 校验时间戳、Severity、Body、Attributes 等关键字段
- 拒绝明显非法、损坏或超出系统限制的请求
- 对非法请求返回明确的 gRPC 错误，而不是吞掉

### 2. 资源属性与上下文字段标准化

应优先将 `spectrum-go-sdk` 上报的资源属性统一映射为稳定字段，包括但不限于：

- `service.name`
- `service.namespace`
- `service.version`
- `service.instance.id`
- `deployment.environment`
- `stellar.cluster`
- `stellar.region`
- `stellar.zone`
- `stellar.idc`
- `host.name`
- `host.ip`
- `k8s.namespace.name`
- `k8s.pod.name`
- `k8s.pod.ip`
- `container.name`

同时应识别并标准化以下请求上下文字段：

- `traceparent`
- `tracestate`
- `baggage`
- `x-stellar-request-id`
- `x-stellar-tenant-id`
- `x-stellar-user-id`
- `x-stellar-session-id`
- `x-stellar-gray-tag`

### 3. 日志语义规范化

- 统一 `SeverityText` 与 `SeverityNumber`
- 统一时间字段时区和精度
- 规范 `body`、异常栈、调用位置和结构化属性的编码方式
- 将空值、重复键、非法键名修正为统一格式
- 为下游补充观测时间、接收时间、来源标识等元信息

### 4. 数据清洗与安全处理

`log agent` 至少应具备以下清洗能力：

- 丢弃或截断超长字段，避免超大消息压垮下游
- 过滤非法 UTF-8、控制字符和脏数据
- 对密码、Token、密钥、身份证号、手机号等敏感信息做脱敏
- 对异常堆栈、错误链和大对象字段做大小限制
- 对不符合规范的高基数字段执行白名单或限制策略

### 5. Kafka 消息整形与路由

在写入 Kafka 前，推荐将日志整理为统一结构化消息，例如：

- 基础身份字段
- 时间字段
- Trace / Request 上下文字段
- 日志正文和结构化属性
- Agent 接收和处理元数据

同时应支持：

- Topic 路由
- 分区键选择
- 压缩策略
- 批量发送
- 失败重试

### 6. 流控、背压与可靠性处理

- 使用有界内存队列，避免内存无限增长
- 当下游写 Kafka 变慢时向上游施加背压
- 对短暂故障做指数退避重试
- 在内存不足或 Kafka 不可用时启用本地磁盘缓冲
- 优雅关闭时尽量 Flush 未发送完的批次

### 7. 自观测能力

`log agent` 必须暴露自己的可观测信号，包括但不限于：

- 接收日志条数
- 校验失败条数
- 清洗失败条数
- Kafka 发送成功条数
- Kafka 发送失败条数
- 当前内存队列长度
- 当前磁盘缓冲积压量
- 被丢弃日志条数
- 重试次数
- 端到端处理延迟

推荐指标命名遵循 `Stellar Axis（星轴）` 统一规范，使用小写英文和下划线命名。

## 高可用与高性能设计建议

### 推荐可靠性语义

默认推荐语义为：

```text
at-least-once
```

原因如下：

- OTLP/gRPC 接收成功并不代表 Kafka 已经落盘
- Kafka 故障恢复、批量重试和本地缓冲回放过程中可能产生重复
- 日志场景通常优先接受“少量重复”，而不是“静默丢失”

因此应明确：

- 优先保证“不轻易丢”
- 接受在极端场景下可能出现少量重复
- 下游消费链路应具备去重或容忍重复的能力

### 推荐处理模型

高性能 `log agent` 推荐采用如下模型：

```text
gRPC receiver
  -> decode / validate workers
  -> normalize / enrich workers
  -> bounded in-memory queue
  -> batcher
  -> Kafka async producer
  -> ack / retry manager
  -> local spool replay worker
```

关键点如下：

- 接收与发送解耦
- 热路径尽量避免重复序列化和深拷贝
- 批量写 Kafka，减少单条发送开销
- 使用异步 Producer 提升吞吐
- 使用有界队列保证系统在高压下仍可控
- 将慢路径，例如磁盘回放、死信处理，与主接收路径分离

### Kafka 写入建议

推荐使用如下策略：

- `acks=all`
- 开启 Producer 重试
- 开启幂等写入能力
- 合理设置 `linger`、`batch.size`、压缩算法
- Broker 地址配置支持多节点
- 对 Topic 不存在、鉴权失败、消息过大等错误做分类处理

### 本地缓冲建议

仅靠内存队列不足以支撑“高可用”，推荐至少支持以下两级缓冲：

1. 内存有界队列
2. 本地磁盘缓冲或 WAL（Write-Ahead Log）

推荐行为：

- 正常情况下优先走内存队列与 Kafka 异步发送
- 当 Kafka 短暂不可用或发送延迟持续升高时，把未确认日志落入本地缓冲
- Agent 重启后优先回放本地缓冲，再恢复正常实时发送

## 丢日志了该怎么处理

日志系统不能只说“尽量不丢”，还要明确“如果丢了怎么补救”。推荐把日志丢失分为以下几类处理。

### 1. 非法日志或毒性日志

典型场景：

- 字段结构损坏
- 序列化失败
- 单条消息超出最大限制
- 含非法字符或无法清洗

处理建议：

- 不要直接静默丢弃
- 返回明确错误并记录拒收原因
- 对样本写入本地隔离文件或死信 Topic
- 增加 `rejected_records_total` 一类指标并触发告警

### 2. Kafka 临时不可用

典型场景：

- Broker 重启
- Leader 切换
- 网络抖动
- Kafka 限流

处理建议：

- Agent 内部执行指数退避重试
- 短期优先堆积在内存队列
- 超过阈值后转入本地磁盘缓冲
- 故障恢复后自动回放磁盘积压

### 3. 下游持续不可用导致积压撑满

典型场景：

- Kafka 长时间不可用
- 本地磁盘也接近打满
- 流量高峰持续超过系统承载

处理建议：

- 先触发背压，降低接收速率
- 再执行降级策略，例如只保留高优先级日志
- 精确统计被丢弃条数、丢弃原因和丢弃时间窗口
- 告警必须包含影响范围，方便后续补偿

### 4. Agent 进程异常退出

处理建议：

- 使用本地 WAL 或 spool 文件保证未发送日志可恢复
- 启动时优先进行断点回放
- 记录上次异常退出恢复结果

### 5. 事后补偿与回放

如果已经确认发生日志丢失，推荐补偿闭环如下：

1. 通过 Agent 指标、错误日志和告警定位丢失时间窗口
2. 从本地磁盘缓冲、隔离文件、应急日志文件中收集待补偿数据
3. 使用专门的 replay 工具按原始格式重新写入 Kafka
4. 为补偿数据打上 `replay=true`、`replay_batch_id` 一类元字段
5. 在下游平台区分实时日志和补偿日志，避免误判

核心原则只有一条：

```text
可以重复，不能静默丢；如果丢了，必须可感知、可定位、可补偿。
```

## 当前模式下：SDK 推日志过来，如果推失败了怎么办

当前链路是：

```text
业务应用 -> spectrum-go-sdk -> localhost:4317 -> spectrum-log-agent
```

在这个模式下，`sdk -> agent` 是本地推送关系，因此失败场景通常集中在：

- 本机 `log agent` 未启动
- 本机 `log agent` 过载
- gRPC 监听不可达
- 请求超时
- Agent 返回限流或错误

针对这些情况，推荐采用分层降级策略。

### 第一层：SDK 内部短暂重试

推荐 SDK 具备以下能力：

- 异步发送
- 小批量聚合
- 有界内存队列
- 指数退避重试
- 单次发送超时控制

适用场景：

- `log agent` 短暂重启
- 本地 gRPC 链路瞬时抖动
- Agent 短时限流

### 第二层：SDK 本地应急缓冲

如果本地 Agent 在短时间内仍不可用，SDK 不应立即把日志全部丢掉，推荐降级到以下方式之一：

1. 本地应急文件
2. 本地 stdout / stderr

其中生产环境更推荐：

```text
本地应急文件（JSON Lines，可滚动切分）
```

原因如下：

- 可被后续 Agent 或 replay 工具重新采集
- 不会和业务控制台输出完全混在一起
- 更容易做丢失补偿

开发环境则可以接受直接降级到：

```text
stdout / stderr
```

### 第三层：Agent 恢复后的补采

如果 SDK 已经降级写入本地应急文件，推荐后续补救方式为：

- Agent 启动后主动扫描应急目录
- 或由单独的 replay 任务回放这些文件
- 回放成功后再归档或删除

### 第四层：最终兜底

如果出现极端情况，例如：

- Agent 不可用
- SDK 队列满
- 本地应急文件不可写
- 业务进程内存也达到上限

则可以允许按优先级丢弃低价值日志，但必须满足以下要求：

- 精确统计丢弃条数
- 打印清晰错误日志
- 暴露丢弃指标
- 触发告警

也就是说，最终兜底可以允许“有损”，但不能“无声”。

## 推荐降级优先级

推荐 `sdk + agent` 组合采用以下降级顺序：

1. 正常发送到本机 `log agent`
2. SDK 短暂重试
3. SDK 写本地应急文件
4. Agent 恢复后自动补采或回放
5. 极端情况下按优先级丢弃低价值日志并告警

不推荐的降级方式：

- SDK 直接绕过 Agent 写 Kafka
- SDK 直接感知远端日志平台拓扑
- 业务代码自己接管失败日志回补逻辑

这样会破坏 `spectrum-go-sdk` 与 `spectrum-log-agent` 之间原本清晰的职责边界。

## 推荐统一 Kafka 日志消息结构

为便于后续消费、索引和分析，推荐 Kafka 中的日志消息至少包含以下字段：

| 字段 | 说明 |
| :--- | :--- |
| `timestamp` | 原始日志时间 |
| `observed_timestamp` | Agent 接收时间 |
| `severity_text` | 日志级别文本 |
| `severity_number` | 日志级别数值 |
| `body` | 日志正文 |
| `trace_id` | Trace ID |
| `span_id` | Span ID |
| `request_id` | 请求 ID |
| `tenant_id` | 租户 ID |
| `user_id` | 用户 ID |
| `service_name` | 服务名 |
| `service_namespace` | 服务命名空间 |
| `service_version` | 服务版本 |
| `service_instance_id` | 服务实例 ID |
| `environment` | 环境 |
| `cluster` | 集群 |
| `region` | 区域 |
| `zone` | 可用区 |
| `host_name` | 主机名 |
| `host_ip` | 主机 IP |
| `pod_name` | Pod 名 |
| `attributes` | 结构化扩展字段 |
| `agent_metadata` | Agent 处理元数据 |

## 推荐指标

建议 `log agent` 至少暴露以下指标：

| 指标名 | 类型 | 说明 |
| :--- | :--- | :--- |
| `logagent_received_records_total` | Counter | 接收日志总数 |
| `logagent_rejected_records_total` | Counter | 校验失败日志数 |
| `logagent_sanitized_records_total` | Counter | 完成清洗的日志数 |
| `logagent_kafka_sent_records_total` | Counter | Kafka 发送成功日志数 |
| `logagent_kafka_send_errors_total` | Counter | Kafka 发送失败次数 |
| `logagent_retry_total` | Counter | 重试次数 |
| `logagent_dropped_records_total` | Counter | 被丢弃日志数 |
| `logagent_inflight_queue_size` | Gauge | 当前内存队列长度 |
| `logagent_spool_backlog_size` | Gauge | 当前磁盘积压量 |
| `logagent_process_duration_ms` | Histogram | 单批处理耗时 |

## 推荐目录结构

考虑到当前项目目标是高可用、高性能 OTLP 日志代理，后续实现建议采用如下目录布局：

```text
spectrum-log-agent/
├── cmd/
│   └── spectrum-log-agent/
│       └── main.go
├── internal/
│   ├── app/
│   ├── config/
│   ├── otlp/
│   ├── receiver/
│   ├── processor/
│   ├── sanitize/
│   ├── buffer/
│   ├── spool/
│   ├── exporter/
│   ├── kafka/
│   ├── metrics/
│   └── shutdown/
├── docs/
└── README.md
```

各目录职责建议如下：

- `receiver`
  负责 OTLP/gRPC 接入
- `processor`
  负责标准化、增强、路由与批处理
- `sanitize`
  负责脱敏、截断、字段清洗
- `buffer`
  负责内存队列和背压控制
- `spool`
  负责本地磁盘缓冲和回放
- `exporter / kafka`
  负责 Kafka Producer 封装与发送
- `metrics`
  负责健康检查和内部指标
- `shutdown`
  负责优雅退出、Flush 与恢复

## 当前 README 范围

由于仓库当前仍处于初始化阶段，本文档先固定以下内容：

- `spectrum-log-agent` 的项目定位
- 与 `spectrum-go-sdk` 的职责边界
- OTLP/gRPC 接收与 Kafka 转发的总体链路
- 本机日志代理应承担的标准处理职责
- 高可用、高性能设计建议
- 日志丢失时的补救方式
- `sdk -> agent` 推送失败时的降级与补偿策略

后续随着代码实现推进，README 可继续补充：

- 配置项说明
- 启动方式
- 本地联调示例
- Kafka Topic 路由规则
- replay 工具使用方式
- 告警与运维手册

## 命名说明

根据 `Stellar Axis（星轴）` 体系统一命名：

- 日志平台名称：`Spectrum · 星谱`
- Go SDK 名称：`spectrum-go-sdk`
- 本机日志代理名称：`spectrum-log-agent`

其中：

- `spectrum` 表示日志平台产品归属
- `log-agent` 表示工程角色

`OpenTelemetry` 作为内部协议和语义实现能力存在，但不进入当前仓库主命名。

## License

Apache License 2.0
