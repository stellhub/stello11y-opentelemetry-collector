package stellspec

const (
	SignalLogs    = "logs"
	SignalMetrics = "metrics"
	SignalTraces  = "traces"
)

type ResourceEnvMapping struct {
	AttributeKey string `mapstructure:"attribute_key"`
	EnvVar       string `mapstructure:"env_var"`
}

type Config struct {
	Signal                  string               `mapstructure:"signal"`
	ProcessorAttributeKey   string               `mapstructure:"processor_attribute_key"`
	ProcessorAttributeValue string               `mapstructure:"processor_attribute_value"`
	KafkaTopic              string               `mapstructure:"kafka_topic"`
	IndexPrefix             string               `mapstructure:"index_prefix"`
	ResourceAttributes      map[string]string    `mapstructure:"resource_attributes"`
	ResourceEnvMappings     []ResourceEnvMapping `mapstructure:"resource_env_mappings"`
	SensitiveAttributeKeys  []string             `mapstructure:"sensitive_attribute_keys"`
	SensitiveReplacement    string               `mapstructure:"sensitive_replacement"`
	MaxAttributeValueBytes  int                  `mapstructure:"max_attribute_value_bytes"`
	MaxLogBodyBytes         int                  `mapstructure:"max_log_body_bytes"`
	MaxSpanNameBytes        int                  `mapstructure:"max_span_name_bytes"`
	MaxMetricAttributes     int                  `mapstructure:"max_metric_attributes"`
	IncludeMetricNames      []string             `mapstructure:"include_metric_names"`
	ExcludeMetricNames      []string             `mapstructure:"exclude_metric_names"`
}

func NewDefaultConfig(signal string) *Config {
	return &Config{
		Signal:                  signal,
		ProcessorAttributeKey:   "stellspec.processor",
		ProcessorAttributeValue: signal,
		KafkaTopic:              "stellspec-" + signal,
		IndexPrefix:             "stellspec-" + signal,
		ResourceEnvMappings:     defaultResourceEnvMappings(),
		SensitiveAttributeKeys:  defaultSensitiveAttributeKeys(),
		SensitiveReplacement:    "[REDACTED]",
		MaxAttributeValueBytes:  2048,
		MaxLogBodyBytes:         65536,
		MaxSpanNameBytes:        512,
		MaxMetricAttributes:     16,
	}
}

func defaultResourceEnvMappings() []ResourceEnvMapping {
	return []ResourceEnvMapping{
		{AttributeKey: "service.name", EnvVar: "STELLAR_APP_NAME"},
		{AttributeKey: "service.namespace", EnvVar: "STELLAR_APP_NAMESPACE"},
		{AttributeKey: "service.version", EnvVar: "STELLAR_APP_VERSION"},
		{AttributeKey: "service.instance.id", EnvVar: "STELLAR_APP_INSTANCE_ID"},
		{AttributeKey: "deployment.environment.name", EnvVar: "STELLAR_ENV"},
		{AttributeKey: "k8s.cluster.name", EnvVar: "STELLAR_CLUSTER"},
		{AttributeKey: "cloud.region", EnvVar: "STELLAR_REGION"},
		{AttributeKey: "cloud.availability_zone", EnvVar: "STELLAR_ZONE"},
		{AttributeKey: "host.name", EnvVar: "STELLAR_HOST_NAME"},
		{AttributeKey: "host.ip", EnvVar: "STELLAR_HOST_IP"},
		{AttributeKey: "k8s.node.name", EnvVar: "STELLAR_NODE_NAME"},
		{AttributeKey: "k8s.namespace.name", EnvVar: "STELLAR_K8S_NAMESPACE"},
		{AttributeKey: "k8s.pod.name", EnvVar: "STELLAR_POD_NAME"},
		{AttributeKey: "k8s.pod.uid", EnvVar: "STELLAR_POD_UID"},
		{AttributeKey: "k8s.pod.ip", EnvVar: "STELLAR_POD_IP"},
		{AttributeKey: "k8s.container.name", EnvVar: "STELLAR_CONTAINER_NAME"},
	}
}

func defaultSensitiveAttributeKeys() []string {
	return []string{
		"authorization",
		"cookie",
		"credential",
		"password",
		"passwd",
		"secret",
		"token",
		"api_key",
		"apikey",
		"access_key",
		"private_key",
	}
}
