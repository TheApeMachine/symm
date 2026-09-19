package transport

import (
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewJSONEncode returns a Value closure that marshals typed input into JSON bytes.
*/
func NewJSONEncode[T any]() types.Value[T, []byte] {
	return func(in T) []byte {
		data, err := sonic.Marshal(in)
		if err != nil {
			errnie.Error(errnie.Err(errnie.Validation, "transport: json marshal failed", err))
			return nil
		}

		return data
	}
}

/*
NewJSONDecode returns a Value closure that unmarshals JSON bytes into a typed output.
*/
func NewJSONDecode[T any]() types.Value[[]byte, T] {
	return func(data []byte) T {
		var out T
		if len(data) == 0 {
			return out
		}

		if err := sonic.Unmarshal(data, &out); err != nil {
			errnie.Error(errnie.Err(errnie.Validation, "transport: json unmarshal failed", err))
			return out
		}

		return out
	}
}
