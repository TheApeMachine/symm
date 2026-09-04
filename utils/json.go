package utils

import (
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

func GetStringSlice(raw []byte, path ...any) []string {
	node, err := sonic.Get(raw, path...)

	if err != nil || !node.Exists() {
		return nil
	}

	value, err := node.Array()

	if err != nil {
		return nil
	}

	items := make([]string, 0, len(value))

	for _, item := range value {
		text, valid := item.(string)

		if !valid {
			return nil
		}

		items = append(items, text)
	}

	return items
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
