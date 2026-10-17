package data

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unsafe"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

/*
Append adds one or more payload layers to Value.
Returns the Value pointer for method chaining.
*/
func (value *Value) Append(layers ...[]byte) *Value {
	for _, layer := range layers {
		value.claim(layer, value.Prefix)
	}

	return value
}

/*
AppendJSON adds a JSON payload layer to Value. The layer owns a copy of buf,
so it never aliases an ingress or ring buffer that gets reused.
*/
func (value *Value) AppendJSON(buf []byte, prefix string) *Value {
	if len(buf) == 0 {
		return value
	}

	layer := make([]byte, len(buf))
	copy(layer, buf)

	value.claim(layer, prefix)

	return value
}

/*
AppendVector stores a typed slice as raw binary bytes on value itself (in place)
and indexes its element keys.
*/
func AppendVector[T any](value *Value, vector []T, keys ...string) *Value {
	if value == nil {
		return nil
	}

	value.claim(vectorBytes(vector), strings.Join(append(
		[]string{value.Prefix}, keys...), "/",
	))

	return value
}

/*
Signal is one signal vector layer of a tick: which source wrote it, for which
symbol, the label of every value, and any trailing "key:value" tags.
*/
type Signal struct {
	Source string
	Symbol string
	Labels []string
	Values []float64
	Tags   []string
}

/*
Metric returns the value labelled label, or ErrMissing.
*/
func (signal Signal) Metric(label string) (float64, error) {
	if idx := slices.Index(signal.Labels, label); idx >= 0 {
		return signal.Values[idx], nil
	}

	return 0, errnie.Err(errnie.NotFound, fmt.Sprintf(
		"data: %s signal for %s has no %s", signal.Source, signal.Symbol, label,
	), ErrMissing)
}

/*
Signals yields every completed signal layer of value (IsSignalIndex), in layer
order, including layers claimed into reserved slots. A nil reserved slot is
still being written and is skipped. A signal layer that does not parse (no
source or symbol, a byte length that is not whole float64s, more values than
labels) is yielded as an error, never skipped.
*/
func Signals(value *Value) iter.Seq2[Signal, error] {
	return func(yield func(Signal, error) bool) {
		if value == nil {
			return
		}

		layers, index := value.filled()

		for i, layer := range layers {
			if !IsSignalIndex(index[i]) {
				continue
			}

			signal, err := ParseSignal(index[i], layer)

			if err != nil {
				err = errnie.Err(errnie.UnprocessableContent, "data: signal layer "+index[i], err)
			}

			if !yield(signal, err) {
				return
			}
		}
	}
}

/*
ParseSignal reads one layer written under SignalIndex.
*/
func ParseSignal(index string, layer []byte) (Signal, error) {
	var signal Signal

	rest, ok := strings.CutPrefix(index, signalNamespace+"/")

	if !ok {
		return signal, errnie.Err(errnie.Validation, "not in the "+signalNamespace+"/ namespace", nil)
	}

	// The symbol tag is last by contract: cut at its last occurrence, so a
	// label or tag containing "/symbol:" stays a label or tag.
	at := strings.LastIndex(rest, "/"+symbolTag)

	if at < 0 || at+len("/"+symbolTag) == len(rest) {
		return signal, errnie.Err(errnie.Validation, "no "+symbolTag+" tag", nil)
	}

	rest, symbol := rest[:at], rest[at+len("/"+symbolTag):]

	parts := strings.Split(rest, "/")

	if parts[0] == "" {
		return signal, errnie.Err(errnie.Validation, "no source", nil)
	}

	signal.Source, signal.Symbol, parts = parts[0], symbol, parts[1:]

	if len(layer)%8 != 0 {
		return signal, errnie.Err(errnie.Validation, fmt.Sprintf("%d bytes is not whole float64s", len(layer)), nil)
	}

	count := len(layer) / 8

	if count > len(parts) {
		return signal, errnie.Err(errnie.Validation, fmt.Sprintf("%d values for %d labels", count, len(parts)), nil)
	}

	if count > 0 {
		signal.Values = slices.Clone(unsafe.Slice((*float64)(unsafe.Pointer(&layer[0])), count))
	}

	signal.Labels = parts[:count]
	signal.Tags = parts[count:]

	return signal, nil
}

func vectorBytes[T any](vector []T) []byte {
	if len(vector) == 0 {
		return []byte{}
	}

	var zero T

	return append([]byte(nil), unsafe.Slice((*byte)(
		unsafe.Pointer(&vector[0]),
	), len(vector)*int(unsafe.Sizeof(zero)))...)
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

	if value == nil || i < 0 || i >= value.visible() {
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
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			fmt.Sprintf("data: layer %d does not decode into %T", i, target),
			err,
		))

		return nil
	}

	return &target
}

/*
Indexed returns the layers, their source, and their indexed metrics.
*/
func Indexed[T any](v *Value, keys ...string) iter.Seq2[string, T] {
	return func(yield func(string, T) bool) {
		if v == nil {
			return
		}
		for i, layer := range v.Layers[:v.visible()] {
			if i >= len(v.Index) || len(v.Index[i]) == 0 || (len(keys) > 0 && !slices.ContainsFunc(keys, func(k string) bool {
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
				val, err := AttributeOk[T](layer, anyKeys...)

				if err != nil {
					if !errors.Is(err, ErrMissing) {
						errnie.Error(err)
					}

					continue
				}

				if reflect.ValueOf(val).IsZero() {
					continue
				}

				out = val
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
		if v == nil {
			return
		}
		for i, layer := range v.Layers[:v.visible()] {
			if len(keys) > 0 && (i >= len(v.Index) || len(v.Index[i]) == 0 || !slices.ContainsFunc(keys, func(k string) bool {
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
				val, err := AttributeOk[T](layer, anyKeys...)

				if err != nil {
					if !errors.Is(err, ErrMissing) {
						errnie.Error(err)
					}

					continue
				}

				if reflect.ValueOf(val).IsZero() {
					continue
				}

				out = val
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

/*
Metrics maps the labels of value's own vector layers to their floats. Signal
layers (per symbol, read with Signals) and JSON layers are not vectors of
the Value and are left out.
*/
func Metrics(v *Value) map[string]float64 {
	out := make(map[string]float64)
	if v == nil {
		return out
	}

	for i, layer := range v.Layers[:v.visible()] {
		if len(layer) < 8 || i >= len(v.Index) {
			continue
		}

		idx := v.Index[i]

		// Signal layers are per symbol; merging them into one map would mix
		// symbols. They read through Signals.
		if IsSignalIndex(idx) {
			continue
		}

		floats := unsafe.Slice((*float64)(unsafe.Pointer(&layer[0])), len(layer)/8)

		// A vector layer is indexed "<key>.bin/<labels...>" (AppendVector on
		// this or another Value). Anything else (JSON layers such as
		// price/<sym>, book/<sym>) is not a labelled vector.
		var afterPrefix string
		if _, after, ok := strings.Cut(idx, ".bin/"); ok {
			afterPrefix = after
		} else if v.Prefix != "" && strings.HasPrefix(idx, v.Prefix+"/") {
			afterPrefix = strings.TrimPrefix(idx, v.Prefix+"/")
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

	return out
}
