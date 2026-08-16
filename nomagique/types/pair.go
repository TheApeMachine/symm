package types

/*
Pair is a descriptive boundary helper. Reducer hot paths represent named values
as interned slots in Frame rather than transporting Pair values.
*/
type Pair[Key comparable, Value comparable] struct {
	Key   Key   `json:"key"`
	Value Value `json:"value"`
}

func NewPair[Key comparable, Value comparable](
	key Key,
	value Value,
) Pair[Key, Value] {
	return Pair[Key, Value]{
		Key:   key,
		Value: value,
	}
}
