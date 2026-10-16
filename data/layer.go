package data

import (
	"fmt"
	"iter"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unsafe"

	"github.com/bytedance/sonic"
)

/*
Append adds one or more payload layers to Value.
Returns the Value pointer for method chaining.
*/
func (value *Value) Append(layers ...[]byte) *Value {
	value.Layers = append(value.Layers, layers...)

	for range layers {
		value.Index = append(value.Index, value.Prefix)
	}

	return value
}

/*
AppendJSON adds a JSON payload layer to Value using sonic.PreallocMarshal.
This is more efficient than Append for large JSON objects.
*/
func (value *Value) AppendJSON(buf []byte, prefix string) *Value {
	value.Layers = append(value.Layers, buf)
	value.Index = append(value.Index, prefix)

	return value
}

/*
AppendVector stores a typed slice as raw binary bytes and indexes its element keys.
*/
func AppendVector[T any](value *Value, vector []T, keys ...string) *Value {
	var zero T

	rawBytes := unsafe.Slice((*byte)(
		unsafe.Pointer(&vector[0]),
	), len(vector)*int(unsafe.Sizeof(zero)))

	value.Layers = append(value.Layers, rawBytes)
	value.Index = append(value.Index, strings.Join(append(
		[]string{value.Prefix}, keys...), "/",
	))

	return value
}

/*
Layer retrieves and unmarshals the payload at the specified layer index into *T.
If idx is omitted, it defaults to Layer 0.
If T is []byte, it returns the raw layer slice directly with zero unmarshaling.
*/
func Layer[T any](value *Value, idx ...int) *T {
	i := 0

	if len(idx) > 0 {
		i = idx[0]
	}

	if value == nil || i < 0 || i >= len(value.Layers) {
		return nil
	}

	layer := value.Layers[i]

	if len(layer) == 0 {
		return nil
	}

	// Zero-copy shortcut if caller requests raw []byte
	if raw, ok := any(layer).(*T); ok {
		return raw
	}

	if raw, ok := any(layer).(T); ok {
		return &raw
	}

	var target T

	if err := sonic.Unmarshal(layer, &target); err != nil {
		return nil
	}

	return &target
}

/*
Indexed returns the layers, their source, and their indexed metrics.
*/
func Indexed[T any](v *Value, keys ...string) iter.Seq2[string, T] {
	return func(yield func(string, T) bool) {
		for i, layer := range v.Layers {
			if i >= len(v.Index) || (len(keys) > 0 && !slices.ContainsFunc(keys, func(k string) bool {
				return strings.Contains(v.Index[i], k)
			})) {
				continue
			}

			var out T
			if raw, ok := any(layer).(T); ok {
				out = raw
			} else if _, ok := any(out).([]float64); ok && len(layer) >= 8 {
				out = any(unsafe.Slice((*float64)(unsafe.Pointer(&layer[0])), len(layer)/8)).(T)
			} else {
				anyKeys := make([]any, len(keys))
				for ki, k := range keys {
					anyKeys[ki] = k
				}
				if val, ok := AttributeOk[T](layer, anyKeys...); ok && !reflect.ValueOf(val).IsZero() {
					out = val
				} else {
					continue
				}
			}

			if !yield(v.Index[i], out) {
				return
			}
		}
	}
}

/*
Layers returns the layers for a given key match.
*/
func Layers[T any](v *Value, keys ...string) iter.Seq[T] {
	return func(yield func(T) bool) {
		for i, layer := range v.Layers {
			if len(keys) > 0 && (i >= len(v.Index) || !slices.ContainsFunc(keys, func(k string) bool {
				return strings.Contains(v.Index[i], k)
			})) {
				continue
			}

			var out T

			if raw, ok := any(layer).(T); ok {
				out = raw
			} else if _, ok := any(out).([]float64); ok && len(layer) >= 8 {
				floats := unsafe.Slice((*float64)(unsafe.Pointer(&layer[0])), len(layer)/8)
				out = any(floats).(T)
			} else {
				anyKeys := make([]any, len(keys))
				for ki, k := range keys {
					anyKeys[ki] = k
				}
				if val, ok := AttributeOk[T](layer, anyKeys...); ok && !reflect.ValueOf(val).IsZero() {
					out = val
				}
			}

			if !yield(out) {
				return
			}
		}
	}
}

/*
Attribute navigates into byte payloads using sonic SIMD path traversal
to pick out a typed attribute without unmarshaling the entire structure.
Handles both native JSON numbers/types and string-encoded venue numbers (e.g. Kraken "123.45").
Returns the zero value of T if the attribute does not exist or cannot be converted.
*/
func Attribute[T any](data []byte, path ...any) T {
	val, _ := AttributeOk[T](data, path...)
	return val
}

/*
AttributeOk navigates into byte payloads using sonic SIMD path traversal
to pick out a typed attribute without unmarshaling the entire structure.
Returns (value, true) if found and converted, or (zero, false) otherwise.
*/
func AttributeOk[T any](data []byte, path ...any) (T, bool) {
	var zero T

	if len(data) == 0 || len(path) == 0 {
		return zero, false
	}

	root, err := sonic.Get(data, path...)
	if err != nil || !root.Exists() {
		return zero, false
	}

	value, err := root.Interface()
	if err != nil || value == nil {
		return zero, false
	}

	return convertLookup[T](value)
}

func convertLookup[T any](value any) (T, bool) {
	if typed, ok := value.(T); ok {
		return typed, true
	}

	if typed, ok := numericPeek[T](value); ok {
		return typed, true
	}

	return slicePeek[T](value)
}

func numericPeek[T any](value any) (T, bool) {
	var zero T

	switch source := value.(type) {
	case float64:
		if math.IsNaN(source) || math.IsInf(source, 0) {
			return zero, false
		}

		switch any(zero).(type) {
		case float64:
			return any(source).(T), true
		case int:
			if math.Trunc(source) == source {
				return any(int(source)).(T), true
			}
		case int64:
			if math.Trunc(source) == source {
				return any(int64(source)).(T), true
			}
		case string:
			return any(strconv.FormatFloat(source, 'f', -1, 64)).(T), true
		}

	case int:
		switch any(zero).(type) {
		case int:
			return any(source).(T), true
		case int64:
			return any(int64(source)).(T), true
		case float64:
			return any(float64(source)).(T), true
		case string:
			return any(strconv.Itoa(source)).(T), true
		}

	case int64:
		switch any(zero).(type) {
		case int64:
			return any(source).(T), true
		case int:
			return any(int(source)).(T), true
		case float64:
			return any(float64(source)).(T), true
		case string:
			return any(strconv.FormatInt(source, 10)).(T), true
		}

	case string:
		trimmed := strings.TrimSpace(source)
		if trimmed == "" {
			return zero, false
		}

		switch any(zero).(type) {
		case float64:
			if f, err := strconv.ParseFloat(trimmed, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
				return any(f).(T), true
			}
		case int:
			if i, err := strconv.Atoi(trimmed); err == nil {
				return any(i).(T), true
			} else if f, err := strconv.ParseFloat(trimmed, 64); err == nil && math.Trunc(f) == f {
				return any(int(f)).(T), true
			}
		case int64:
			if i, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
				return any(i).(T), true
			} else if f, err := strconv.ParseFloat(trimmed, 64); err == nil && math.Trunc(f) == f {
				return any(int64(f)).(T), true
			}
		case bool:
			if b, err := strconv.ParseBool(trimmed); err == nil {
				return any(b).(T), true
			}
		}

	case bool:
		switch any(zero).(type) {
		case bool:
			return any(source).(T), true
		case string:
			return any(strconv.FormatBool(source)).(T), true
		}
	}

	return zero, false
}

func slicePeek[T any](value any) (T, bool) {
	var zero T

	items, ok := value.([]any)
	if !ok {
		return zero, false
	}

	switch any(zero).(type) {
	case []string:
		values := make([]string, len(items))
		for index, item := range items {
			switch typed := item.(type) {
			case string:
				values[index] = typed
			case float64:
				values[index] = strconv.FormatFloat(typed, 'f', -1, 64)
			default:
				values[index] = fmt.Sprint(item)
			}
		}
		return any(values).(T), true

	case []float64:
		values := make([]float64, len(items))
		for index, item := range items {
			switch typed := item.(type) {
			case float64:
				values[index] = typed
			case int:
				values[index] = float64(typed)
			case int64:
				values[index] = float64(typed)
			case string:
				if f, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
					values[index] = f
				} else {
					return zero, false
				}
			default:
				return zero, false
			}
		}
		return any(values).(T), true
	}

	return zero, false
}

/*
Metrics extracts all indexed vector metric labels and attributes into a map.
*/
func Metrics(v *Value) map[string]float64 {
	out := make(map[string]float64)
	if v == nil {
		return out
	}

	for i, layer := range v.Layers {
		if len(layer) < 8 || i >= len(v.Index) {
			continue
		}

		floats := unsafe.Slice((*float64)(unsafe.Pointer(&layer[0])), len(layer)/8)
		idx := v.Index[i]

		afterPrefix := idx
		if _, after, ok := strings.Cut(idx, ".bin/"); ok {
			afterPrefix = after
		} else if v.Prefix != "" && strings.HasPrefix(idx, v.Prefix) {
			afterPrefix = strings.TrimPrefix(idx, v.Prefix)
			afterPrefix = strings.TrimPrefix(afterPrefix, "/")
		} else {
			parts := strings.Split(idx, "/")
			if len(parts) > 5 {
				afterPrefix = strings.Join(parts[5:], "/")
			}
		}

		if afterPrefix == "" {
			continue
		}

		labels := strings.Split(afterPrefix, "/")
		for k, f := range floats {
			if k < len(labels) {
				out[labels[k]] = f
			}
		}
	}

	if _, ok := out["price"]; !ok && len(v.Layers) > 0 {
		first := v.Layers[0]
		if len(first) > 0 && (first[0] == '{' || first[0] == '[') {
			if p := Attribute[float64](first, "data", 0, "price"); p > 0 {
				out["price"] = p
			}
		}
	}

	return out
}
