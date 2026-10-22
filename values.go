package config

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Limites da plataforma para conversões numéricas seguras.
var (
	maxIntValue = int(^uint(0) >> 1)
	minIntValue = -maxIntValue - 1
)

// lookup navega mapas aninhados usando uma chave em notação de ponto.
func lookup(root map[string]any, key string) (any, bool) {
	if root == nil || key == "" {
		return nil, false
	}
	var current any = root
	for _, part := range strings.Split(key, ".") {
		if part == "" {
			return nil, false
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := m[part]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

// toInt converte os tipos numéricos usuais e strings numéricas em int.
func toInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return intFromInt64(v)
	case uint:
		return intFromUint64(uint64(v))
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return intFromUint64(v)
	case float32:
		return floatToInt(float64(v))
	case float64:
		return floatToInt(v)
	case json.Number:
		if n, err := strconv.ParseInt(v.String(), 10, 64); err == nil {
			return intFromInt64(n)
		}
		if f, err := strconv.ParseFloat(v.String(), 64); err == nil {
			return floatToInt(f)
		}
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return intFromInt64(n)
		}
	}
	return 0, false
}

// intFromInt64 converte um int64 para int apenas quando não há perda.
func intFromInt64(value int64) (int, bool) {
	converted := int(value)
	if int64(converted) != value {
		return 0, false
	}
	return converted, true
}

// intFromUint64 converte um uint64 para int apenas quando não há perda.
func intFromUint64(value uint64) (int, bool) {
	if value > uint64(maxIntValue) {
		return 0, false
	}
	return int(value), true
}

// floatToInt converte um float integral que caiba em int.
func floatToInt(value float64) (int, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	if value < float64(minIntValue) || value >= float64(maxIntValue) {
		return 0, false
	}
	return int(value), true
}

// toBool converte booleanos e representações textuais usuais em bool.
func toBool(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "y", "on":
			return true, true
		case "false", "0", "no", "n", "off":
			return false, true
		}
	case int:
		return intToBool(int64(v))
	case int8:
		return intToBool(int64(v))
	case int16:
		return intToBool(int64(v))
	case int32:
		return intToBool(int64(v))
	case int64:
		return intToBool(v)
	case uint:
		return uintToBool(uint64(v))
	case uint8:
		return uintToBool(uint64(v))
	case uint16:
		return uintToBool(uint64(v))
	case uint32:
		return uintToBool(uint64(v))
	case uint64:
		return uintToBool(v)
	case json.Number:
		if n, err := strconv.ParseInt(v.String(), 10, 64); err == nil {
			return intToBool(n)
		}
	}
	return false, false
}

// intToBool aceita apenas 0 e 1.
func intToBool(value int64) (bool, bool) {
	switch value {
	case 0:
		return false, true
	case 1:
		return true, true
	default:
		return false, false
	}
}

// uintToBool aceita apenas 0 e 1.
func uintToBool(value uint64) (bool, bool) {
	switch value {
	case 0:
		return false, true
	case 1:
		return true, true
	default:
		return false, false
	}
}

// toDuration converte durações, strings no formato de time.ParseDuration e
// números (nanossegundos) em time.Duration.
func toDuration(value any) (time.Duration, bool) {
	switch v := value.(type) {
	case time.Duration:
		return v, true
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, false
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return 0, false
		}
		return d, true
	case int:
		return time.Duration(v), true
	case int8:
		return time.Duration(v), true
	case int16:
		return time.Duration(v), true
	case int32:
		return time.Duration(v), true
	case int64:
		return time.Duration(v), true
	case uint:
		return durationFromUint64(uint64(v))
	case uint8:
		return time.Duration(v), true
	case uint16:
		return time.Duration(v), true
	case uint32:
		return time.Duration(v), true
	case uint64:
		return durationFromUint64(v)
	case float32:
		return durationFromFloat(float64(v))
	case float64:
		return durationFromFloat(v)
	case json.Number:
		if n, err := strconv.ParseInt(v.String(), 10, 64); err == nil {
			return time.Duration(n), true
		}
		if f, err := strconv.ParseFloat(v.String(), 64); err == nil {
			return durationFromFloat(f)
		}
	}
	return 0, false
}

// durationFromUint64 converte nanossegundos sem estourar int64.
func durationFromUint64(value uint64) (time.Duration, bool) {
	if value > math.MaxInt64 {
		return 0, false
	}
	return time.Duration(value), true
}

// durationFromFloat converte nanossegundos integrais que caibam em int64.
func durationFromFloat(value float64) (time.Duration, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	if value < float64(math.MinInt64) || value >= float64(math.MaxInt64) {
		return 0, false
	}
	return time.Duration(value), true
}

// isList informa se o valor é um slice ou array.
func isList(value any) bool {
	if value == nil {
		return false
	}
	kind := reflect.ValueOf(value).Kind()
	return kind == reflect.Slice || kind == reflect.Array
}

// formatValue representa um valor para exibição sem iterar mapas fora de
// ordem. Mapas e structs usam JSON, que ordena as chaves.
func formatValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "—"
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case time.Duration:
		return v.String()
	case json.Number:
		return v.String()
	case []string:
		return formatList(v)
	case []any:
		return formatList(v)
	}

	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		return formatList(value)
	case reflect.Map, reflect.Struct, reflect.Ptr, reflect.Interface:
		if data, err := json.Marshal(value); err == nil {
			return string(data)
		}
		return "<valor>"
	case reflect.String:
		return rv.String()
	default:
		return fmt.Sprint(value)
	}
}

// formatList representa uma lista em uma única linha.
func formatList(value any) string {
	if !isList(value) {
		return formatValue(value)
	}
	rv := reflect.ValueOf(value)
	parts := make([]string, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		parts = append(parts, formatListElement(rv.Index(i).Interface()))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// formatListElement exibe strings sem aspas e os demais valores normalmente.
func formatListElement(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return formatValue(value)
}
