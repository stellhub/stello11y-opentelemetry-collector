package stellspec

import (
	"net/url"
	"os"
	"strings"
)

type Runtime struct {
	cfg              *Config
	resourceDefaults map[string]string
	includeMetrics   map[string]struct{}
	excludeMetrics   map[string]struct{}
}

func NewRuntime(cfg *Config) *Runtime {
	return &Runtime{
		cfg:              cfg,
		resourceDefaults: buildResourceDefaults(cfg),
		includeMetrics:   stringSet(cfg.IncludeMetricNames),
		excludeMetrics:   stringSet(cfg.ExcludeMetricNames),
	}
}

func buildResourceDefaults(cfg *Config) map[string]string {
	defaults := parseOTELResourceAttributes(os.Getenv("OTEL_RESOURCE_ATTRIBUTES"))

	if serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME")); serviceName != "" {
		defaults["service.name"] = serviceName
	}

	for _, mapping := range cfg.ResourceEnvMappings {
		attr := strings.TrimSpace(mapping.AttributeKey)
		envVar := strings.TrimSpace(mapping.EnvVar)
		if attr == "" || envVar == "" {
			continue
		}
		if _, exists := defaults[attr]; exists {
			continue
		}
		if value := strings.TrimSpace(os.Getenv(envVar)); value != "" {
			defaults[attr] = value
		}
	}

	for key, value := range cfg.ResourceAttributes {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		if _, exists := defaults[key]; exists {
			continue
		}
		defaults[key] = value
	}

	return defaults
}

func parseOTELResourceAttributes(raw string) map[string]string {
	defaults := make(map[string]string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaults
	}

	for _, pair := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if decoded, err := url.PathUnescape(value); err == nil {
			value = decoded
		}
		if value != "" {
			defaults[key] = value
		}
	}

	return defaults
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if normalized := strings.TrimSpace(value); normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	return set
}
