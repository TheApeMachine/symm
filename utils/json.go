package utils

import (
	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/theapemachine/errnie"
)

func UnmarshalSlice[T any](raw []byte) []T {
	var v []T

	err := sonic.Unmarshal(raw, &v)

	if err != nil {
		return nil
	}

	return v
}

func Unmarshal[T any](raw []byte) T {
	var v T

	err := sonic.Unmarshal(raw, &v)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"json: unmarshal failed",
			err,
		))

		return v
	}

	return v
}

func GetStringSlice(raw []byte, path ...any) []string {
	node, err := sonic.Get(raw, path...)

	if err != nil || !node.Exists() {
		return nil
	}

	value, err := node.Array()

	if err != nil {
		return nil
	}

	out := make([]string, len(value))

	for _, v := range value {
		out = append(out, v.(string))
	}

	return out
}

func GetString(raw []byte, path ...any) string {
	node, err := sonic.Get(raw, path...)

	if err != nil || !node.Exists() {
		return ""
	}

	value, err := node.String()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))

		return ""
	}

	return value
}

/*
GetBytes returns the raw JSON at path. Missing paths return nil, nil so callers
can probe keys without treating absence as a fault.
*/
func GetBytes(raw []byte, path ...any) ([]byte, error) {
	node, err := sonic.Get(raw, path...)

	if err != nil || !node.Exists() {
		return nil, nil
	}

	value, err := node.Raw()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"json: path raw decode failed",
			err,
		))
	}

	if value == "" {
		return nil, nil
	}

	return []byte(value), nil
}

func FrameData(raw []byte) ([]byte, error) {
	mapped, err := kraken.NewWebSocketMessage(raw).Map()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"json: frame data map failed",
			err,
		))
	}

	return sonic.Marshal(mapped)
}
