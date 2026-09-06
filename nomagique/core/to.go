package core

// To reads at a Go boundary. Primitive-valued folds retain the actual yielded
// object rather than decoding its payload. Computational packages use Yield.
func To[T any](value Primitive) T {
	var zero T
	if value == nil {
		return zero
	}
	if _, opaque := any((*T)(nil)).(*Primitive); opaque {
		return any(value).(T)
	}
	readout := value.Read()
	if readout == nil {
		if _, untyped := any((*T)(nil)).(*any); untyped {
			return zero
		}
		value.Error(ErrNotHeld)
		return zero
	}
	result, ok := readout.(T)
	if !ok {
		value.Error(ErrConversion)
	}
	return result
}
