package config

import (
	"reflect"
	"strings"
)

// mergeMaps devolve um novo mapa com src aplicado sobre dst.
//
// Mapas são mesclados recursivamente; qualquer outro valor, inclusive listas,
// substitui o valor anterior. O resultado não compartilha mapas nem slices com
// os argumentos.
func mergeMaps(dst, src map[string]any) map[string]any {
	out := cloneMap(dst)
	if out == nil {
		out = make(map[string]any)
	}
	for key, value := range src {
		if child, ok := value.(map[string]any); ok {
			current, _ := out[key].(map[string]any)
			out[key] = mergeMaps(current, child)
			continue
		}
		out[key] = cloneValue(value)
	}
	return out
}

// cloneMap devolve uma cópia profunda de um mapa de configuração.
func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for key, value := range m {
		out[key] = cloneValue(value)
	}
	return out
}

// cloneValue copia mapas e slices; outros escalares são devolvidos como estão.
func cloneValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneMap(v)
	case []any:
		if v == nil {
			return []any(nil)
		}
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneValue(item)
		}
		return out
	case []string:
		if v == nil {
			return []string(nil)
		}
		out := make([]string, len(v))
		copy(out, v)
		return out
	}

	rv := reflect.ValueOf(value)
	if rv.IsValid() && rv.Kind() == reflect.Slice {
		if rv.IsNil() {
			return value
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			item := cloneValue(rv.Index(i).Interface())
			if item == nil {
				out.Index(i).Set(reflect.Zero(rv.Type().Elem()))
				continue
			}
			cloned := reflect.ValueOf(item)
			if !cloned.Type().AssignableTo(rv.Type().Elem()) {
				cloned = rv.Index(i)
			}
			out.Index(i).Set(cloned)
		}
		return out.Interface()
	}
	if rv.IsValid() && rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
		if rv.IsNil() {
			return value
		}
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = cloneValue(iter.Value().Interface())
		}
		return out
	}
	return value
}

// setPath grava um valor em um mapa aninhado, criando os mapas necessários.
func setPath(root map[string]any, key string, value any) {
	if root == nil || key == "" {
		return
	}
	parts := strings.Split(key, ".")
	for i, part := range parts {
		if part == "" {
			return
		}
		if i == len(parts)-1 {
			root[part] = value
			return
		}
		child, ok := root[part].(map[string]any)
		if !ok {
			child = make(map[string]any)
			root[part] = child
		}
		root = child
	}
}
