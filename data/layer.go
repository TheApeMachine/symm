package data

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"iter"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

/*
Append stores layers on value as one gob blob, addressed by keys.
The address is value.Prefix followed by keys, joined with "/".
Layer reads it back with T set to the same slice type, as in
Layer[[]Trade](value, "trade") after Append(value, []Trade{...}, "trade").
*/
func (value *Value) Append(layers any, keys ...string) *Value {
	var buffer bytes.Buffer

	if err := gob.NewEncoder(&buffer).Encode(layers); err != nil {
		value.Err = errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[data.Append] layer does not encode",
			err,
		))

		return value
	}

	value.appendLayer(buffer.Bytes(), strings.Join(append([]string{value.Prefix}, keys...), "/"))

	return value
}

/*
Lookup finds a layer by keyword.
*/
func (value *Value) Lookup(projector any, keys ...string) {
	for i, prefix := range value.Index {
		for _, key := range keys {
			if strings.Contains(prefix, key) {
				decoder := gob.NewDecoder(
					bytes.NewReader(value.Layers[i]),
				)

				if err := decoder.Decode(projector); err != nil {
					value.Err = errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"[data.Lookup] "+key+" does not decode",
						err,
					))

					return
				}
			}
		}
	}
}

/*
Layer yields each gob layer whose index contains prefix as a whole path
segment. T is the type Append stored. A blob that does not decode is
recorded on value.Err and the sequence stops. No prefix yields every layer.
*/
func Layer[T any](value *Value, prefixes ...string) iter.Seq2[string, T] {
	return func(yield func(string, T) bool) {
		if value == nil {
			return
		}

		layers, index := value.filled()
		matched := false

		for i, layer := range layers {
			if len(prefixes) > 0 && !slices.ContainsFunc(prefixes, func(prefix string) bool {
				return segmentContains(index[i], prefix)
			}) {
				continue
			}

			var out T

			if err := gob.NewDecoder(bytes.NewReader(layer)).Decode(&out); err != nil {
				packed, ok := packedFloat64[T](layer)

				if !ok {
					value.Err = errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"[data.Layer] "+index[i]+" does not decode",
						err,
					))

					return
				}

				out = packed
			}

			matched = true

			if !yield(index[i], out) {
				return
			}
		}

		if len(prefixes) > 0 && !matched && value.Err == nil {
			value.Err = errnie.Error(errnie.Err(
				errnie.NotFound,
				"[data.Layer] "+strings.Join(prefixes, "/")+" is not on "+value.Prefix,
				nil,
			))
		}
	}
}

/*
packedFloat64 reads a little-endian float64 vector. Stored ticks and
detections keep that layout. Append stores the same vector as gob.
*/
func packedFloat64[T any](layer []byte) (T, bool) {
	var zero T

	if _, ok := any(zero).([]float64); !ok || len(layer) == 0 || len(layer)%8 != 0 {
		return zero, false
	}

	floats := make([]float64, len(layer)/8)

	for i := range floats {
		floats[i] = math.Float64frombits(binary.LittleEndian.Uint64(layer[i*8 : i*8+8]))
	}

	packed, ok := any(floats).(T)

	return packed, ok
}

/*
segmentContains reports whether index holds key as a whole path segment,
so book/BTC/USD does not match book/BTC/USDT.
*/
func segmentContains(index, key string) bool {
	if key == "" || index == "" {
		return false
	}

	from := 0

	for {
		at := strings.Index(index[from:], key)

		if at < 0 {
			return false
		}

		at += from
		end := at + len(key)
		before := at == 0 || index[at-1] == '/'
		after := end == len(index) || index[end] == '/' || strings.HasSuffix(key, "/")

		if before && after {
			return true
		}

		from = at + 1
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
