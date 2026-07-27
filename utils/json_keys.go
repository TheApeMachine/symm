package utils

import (
	"unsafe"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

/*
EachKey walks top-level object keys once and yields each key with its raw JSON
value. Used by the UI hub so fat frames are not re-searched per cache key.
*/
func EachKey(raw []byte, visit func(key string, value []byte) bool) error {
	if len(raw) == 0 || visit == nil {
		return nil
	}

	root, err := sonic.Get(raw)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"json: object root lookup failed",
			err,
		))
	}

	properties, err := root.Properties()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"json: object properties failed",
			err,
		))
	}

	var pair ast.Pair

	for properties.Next(&pair) {
		value, err := pair.Value.Raw()

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"json: object value raw failed",
				err,
			))
		}

		valBytes := unsafe.Slice(unsafe.StringData(value), len(value))

		if !visit(pair.Key, valBytes) {
			return nil
		}
	}

	return nil
}
