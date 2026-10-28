package data

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

func (value *Value) GetString(key string) string {
	var str string
	value.Lookup(&str, key)
	return str
}

func (value *Value) GetFloat64(key string) float64 {
	var f float64
	value.Lookup(&f, key)
	return f
}

func Get[T any](value *Value, key string) T {
	var t T
	value.Lookup(&t, key)
	return t
}

/*
GetTotal returns the sum of all values for the given key.
*/
func (value *Value) GetTotal(key string) float64 {
	if value == nil {
		return 0
	}

	total := 0.0
	target := strings.Join([]string{value.Prefix, key}, "/")

	for i, index := range value.Index {
		if index != target && !strings.HasPrefix(index, target+"/") {
			continue
		}

		var f float64

		if err := sonic.Unmarshal(value.Layers[i], &f); err == nil {
			total += f
		}
	}

	return total
}

/*
Append stores layers on value as one gob blob, addressed by keys.
The address is value.Prefix followed by keys, joined with "/".
Layer reads it back with T set to the same slice type, as in
Layer[[]Trade](value, "trade") after Append(value, []Trade{...}, "trade").
*/
func (value *Value) Append(layers any, keys ...string) *Value {
	value.mu.Lock()
	defer value.mu.Unlock()

	buffer, err := sonic.Marshal(layers)

	if err != nil {
		value.Err = errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[data.Append] layer does not encode",
			err,
		))
		return value
	}

	value.Layers = append(value.Layers, buffer)
	value.Index = append(value.Index, strings.Join(append(
		[]string{value.Prefix}, keys...), "/",
	))

	return value
}

/*
AppendBytes stores an already-encoded layer without re-marshaling (S3 list frames).
*/
func (value *Value) AppendBytes(layer []byte, keys ...string) *Value {
	value.mu.Lock()
	defer value.mu.Unlock()

	if len(layer) == 0 {
		return value
	}

	value.Layers = append(value.Layers, append([]byte(nil), layer...))
	value.Index = append(value.Index, strings.Join(append(
		[]string{value.Prefix}, keys...), "/",
	))

	return value
}

/*
Set sets the value at the given key to the given value.
*/
func (value *Value) Set(layer any, keys ...string) {
	value.mu.Lock()
	defer value.mu.Unlock()

	found := false
	target := strings.Join([]string{value.Prefix, strings.Join(keys, "/")}, "/")
	buffer, err := sonic.Marshal(layer)

	if err != nil {
		value.Err = errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[data.Set] value does not encode",
			err,
		))
	}

	for i, index := range value.Index {
		if index != target {
			continue
		}

		value.Layers[i] = buffer
		found = true
	}

	if !found {
		value.Index = append(value.Index, target)
		value.Layers = append(value.Layers, buffer)
	}
}

/*
At
*/
func (value *Value) At(projector any, index int) {
	value.mu.RLock()
	defer value.mu.RUnlock()

	if err := sonic.Unmarshal(value.Layers[index], projector); err != nil {
		value.Err = errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[data.Value.At] layer does not decode",
			err,
		))
	}
}

/*
Lookup decodes the first layer addressed by key relative to value.Prefix.

Matching is prefix-safe: index == prefix/key or index begins with prefix/key/
(so spot:trade/trade resolves Lookup(..., "spot:trade")). Sibling keys such
as policyPath must not match Lookup(..., "policy"); use LookupPrefix when
you need every descendant under a path segment.
*/
func (value *Value) Lookup(projector any, key string) {
	value.mu.RLock()
	defer value.mu.RUnlock()

	var buf []byte

	target := strings.Join([]string{value.Prefix, key}, "/")

	for i, index := range value.Index {
		if index != target && !strings.HasPrefix(index, target+"/") {
			continue
		}

		buf = value.Layers[i]
		break
	}

	if buf == nil || len(bytes.TrimSpace(buf)) == 0 {
		return
	}

	if err := sonic.Unmarshal(buf, projector); err != nil {
		value.Err = errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[data.Lookup] "+key+" does not decode",
			err,
		))
	}
}

/*
All returns all layers addressed by key.
*/
func (value *Value) All(projector any, key string) {
	value.mu.RLock()
	defer value.mu.RUnlock()

	all := make([][]byte, 0)
	target := strings.Join([]string{value.Prefix, key}, "/")

	for i, index := range value.Index {
		if index != target && !strings.HasPrefix(index, target+"/") {
			continue
		}

		all = append(all, value.Layers[i])
	}

	if err := sonic.Unmarshal(bytes.Join(all, []byte("\n")), projector); err != nil {
		value.Err = errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[data.All] layer does not decode",
			err,
		))
	}
}

/*
Merged returns the value's layers keyed by index. The value is not changed.
*/
func (value *Value) Merged() map[string][]byte {
	value.mu.RLock()
	defer value.mu.RUnlock()

	merged := make(map[string][]byte, len(value.Index))

	for i, index := range value.Index {
		if i < len(value.Layers) {
			merged[index] = value.Layers[i]
		}
	}

	return merged
}

/*
With returns a copy of value carrying exactly the layers addressed by keys
in order. It is how a frame says "these layers, with exactly these bytes"
without mutating the source.
*/
func (value *Value) With(keys ...string) *Value {
	clone := &Value{
		ID:        value.ID,
		Prefix:    value.Prefix,
		Epoch:     value.Epoch,
		Tick:      value.Tick,
		SeqIdx:    value.SeqIdx,
		Timestamp: value.Timestamp,
		Origin:    value.Origin,
		Role:      value.Role,
		Scope:     value.Scope,
	}

	byIndex := value.Merged()

	for _, key := range keys {
		if buf, ok := byIndex[key]; ok {
			clone.Index = append(clone.Index, key)
			clone.Layers = append(clone.Layers, buf)
		}
	}

	return clone
}

type LayerEntry struct {
	Key  string // Relative to the requested prefix.
	Data []byte
}

func (layer *LayerEntry) Decode(projector any) error {
	if layer == nil || projector == nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[data.LayerEntry.Decode] nil layer or projector",
			nil,
		)
	}

	if err := sonic.Unmarshal(layer.Data, projector); err != nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[data.LayerEntry.Decode] layer does not decode",
			err,
		)
	}

	return nil
}

/*
LookupPrefix returns all descendant layers below key.
For example, LookupPrefix("cvd") returns each CVD layer,
with keys such as "BTC/USD/midpoint_log_return/...".
*/
func (value *Value) LookupPrefix(key string) []LayerEntry {
	value.mu.RLock()
	defer value.mu.RUnlock()

	if value == nil {
		return nil
	}

	index := append([]string(nil), value.Index...)
	layers := make([][]byte, len(value.Layers))
	copy(layers, value.Layers)
	prefix := value.Prefix

	target := prefix
	if key != "" {
		target = strings.Join([]string{prefix, key}, "/")
	}

	entries := make([]LayerEntry, 0, len(index))
	trim := prefix + "/"

	for i, layerIndex := range index {
		if i >= len(layers) {
			continue
		}

		if key == "" {
			if layerIndex != prefix && !strings.HasPrefix(layerIndex, trim) {
				continue
			}
		} else if layerIndex != target && !strings.HasPrefix(layerIndex, target+"/") {
			continue
		}

		entries = append(entries, LayerEntry{
			Key:  strings.TrimPrefix(layerIndex, trim),
			Data: layers[i],
		})
	}

	return entries
}

/*
Attribute navigates into byte payloads using sonic SIMD path traversal
to pick out a typed attribute without unmarshaling the entire structure.
Handles both native JSON numbers/types and string-encoded venue numbers (e.g. Kraken "123.45").
Returns the zero value of T if the attribute does not exist; a value that
cannot be converted is logged. Use AttributeOk to receive the error.
*/
func Attribute[T any](data []byte, path ...any) T {
	val, err := AttributeOk[T](data, path...)

	if err != nil && !errors.Is(err, ErrMissing) {
		errnie.Error(err)
	}

	return val
}

/*
ErrMissing reports that a path does not exist in a payload (or holds null).
It is the one expected outcome of a lookup; every other failure is an error.
*/
var ErrMissing = errors.New("data: attribute missing")

/*
AttributeOk picks the attribute at path out of data as T using sonic SIMD
path traversal. Scalars and slices of scalars use the venue-number
conversions; any other T (structs, slices of structs) is unmarshaled from the
attribute's raw JSON. A missing path returns ErrMissing; a value that does not
decode into T returns that error. It never returns a zero value with a nil
error.
*/
func AttributeOk[T any](data []byte, path ...any) (T, error) {
	var zero T

	if len(data) == 0 || len(path) == 0 {
		return zero, ErrMissing
	}

	root, err := sonic.Get(data, path...)

	if errors.Is(err, ast.ErrNotExist) {
		return zero, ErrMissing
	}

	if err != nil {
		return zero, errnie.Err(
			errnie.UnprocessableContent,
			fmt.Sprintf("data: attribute %v unreadable", path),
			err,
		)
	}

	value, err := root.Interface()

	if err != nil {
		return zero, errnie.Err(
			errnie.UnprocessableContent,
			fmt.Sprintf("data: attribute %v unreadable", path),
			err,
		)
	}

	if value == nil {
		return zero, ErrMissing
	}

	if converted, ok := convertLookup[T](value); ok {
		return converted, nil
	}

	raw, err := root.Raw()

	if err != nil {
		return zero, errnie.Err(
			errnie.UnprocessableContent,
			fmt.Sprintf("data: attribute %v unreadable", path),
			err,
		)
	}

	var out T

	if err := sonic.UnmarshalString(raw, &out); err != nil {
		return zero, errnie.Err(
			errnie.UnprocessableContent,
			fmt.Sprintf("data: attribute %v not decodable as %T", path, zero),
			err,
		)
	}

	return out, nil
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
