package stellspec

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

var (
	sensitiveJSONPattern  = regexp.MustCompile(`(?i)("?(authorization|cookie|password|passwd|secret|token|api[_-]?key|access[_-]?key)"?\s*:\s*")([^"]*)(")`)
	sensitivePairPattern  = regexp.MustCompile(`(?i)((authorization|cookie|password|passwd|secret|token|api[_-]?key|access[_-]?key)\s*=\s*)([^,\s}]+)`)
	sensitiveColonPattern = regexp.MustCompile(`(?i)((authorization|cookie|password|passwd|secret|token|api[_-]?key|access[_-]?key)\s*:\s*)([^,\s}]+)`)
)

func (rt *Runtime) NormalizeResource(attrs pcommon.Map) {
	for key, value := range rt.resourceDefaults {
		putStrIfMissing(attrs, key, value)
	}

	putStrIfMissing(attrs, rt.cfg.ProcessorAttributeKey, rt.cfg.ProcessorAttributeValue)
	putStrIfMissing(attrs, "stellspec.signal", rt.cfg.Signal)
	putStrIfMissing(attrs, "stellspec.kafka.topic", rt.cfg.KafkaTopic)
	putStrIfMissing(attrs, "stellspec.index.prefix", rt.cfg.IndexPrefix)

	rt.SanitizeMap(attrs)
}

func (rt *Runtime) SanitizeMap(attrs pcommon.Map) {
	if attrs.Len() == 0 {
		return
	}

	removeKeys := make([]string, 0)
	attrs.Range(func(key string, value pcommon.Value) bool {
		if rt.isSensitiveKey(key) {
			if rt.cfg.SensitiveReplacement == "" {
				removeKeys = append(removeKeys, key)
			} else {
				value.SetStr(rt.cfg.SensitiveReplacement)
			}
			return true
		}

		rt.SanitizeValue(value)
		return true
	})

	for _, key := range removeKeys {
		attrs.Remove(key)
	}
}

func (rt *Runtime) SanitizeValue(value pcommon.Value) {
	switch value.Type() {
	case pcommon.ValueTypeStr:
		s := RedactSensitiveText(value.Str(), rt.cfg.SensitiveReplacement)
		value.SetStr(truncateUTF8(s, rt.cfg.MaxAttributeValueBytes))
	case pcommon.ValueTypeMap:
		rt.SanitizeMap(value.Map())
	case pcommon.ValueTypeSlice:
		slice := value.Slice()
		for i := 0; i < slice.Len(); i++ {
			rt.SanitizeValue(slice.At(i))
		}
	}
}

func RedactSensitiveText(value, replacement string) string {
	if value == "" {
		return value
	}
	if replacement == "" {
		replacement = "[REDACTED]"
	}
	value = sensitiveJSONPattern.ReplaceAllString(value, "${1}"+replacement+"${4}")
	value = sensitivePairPattern.ReplaceAllString(value, "${1}"+replacement)
	value = sensitiveColonPattern.ReplaceAllString(value, "${1}"+replacement)
	return value
}

func (rt *Runtime) isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, sensitiveKey := range rt.cfg.SensitiveAttributeKeys {
		if sensitiveKey == "" {
			continue
		}
		if strings.Contains(normalized, strings.ToLower(sensitiveKey)) {
			return true
		}
	}
	return false
}

func putStrIfMissing(attrs pcommon.Map, key, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	if existing, ok := attrs.Get(key); ok && valueExists(existing) {
		return
	}
	attrs.PutStr(key, value)
}

func valueExists(value pcommon.Value) bool {
	switch value.Type() {
	case pcommon.ValueTypeEmpty:
		return false
	case pcommon.ValueTypeStr:
		return strings.TrimSpace(value.Str()) != ""
	case pcommon.ValueTypeMap:
		return value.Map().Len() > 0
	case pcommon.ValueTypeSlice:
		return value.Slice().Len() > 0
	default:
		return true
	}
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}

	truncated := value[:maxBytes]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}

func attrInt(attrs pcommon.Map, keys ...string) (int64, bool) {
	for _, key := range keys {
		if value, ok := attrs.Get(key); ok {
			switch value.Type() {
			case pcommon.ValueTypeInt:
				return value.Int(), true
			case pcommon.ValueTypeStr:
				if parsed, ok := parseInt(value.Str()); ok {
					return parsed, true
				}
			}
		}
	}
	return 0, false
}

func attrString(attrs pcommon.Map, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := attrs.Get(key); ok {
			if s := strings.TrimSpace(value.AsString()); s != "" {
				return s, true
			}
		}
	}
	return "", false
}

func parseInt(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	var result int64
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
		result = result*10 + int64(r-'0')
	}
	return result, true
}
