package data

import (
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"
)

var CurrentValue = &atomic.Pointer[*Value]{}

type Value struct {
	ID        [16]byte
	Prefix    string
	Epoch     uint64
	Tick      uint64
	SeqIdx    uint64
	Timestamp uint64
	Origin    string
	Role      string
	Scope     string
	Layers    [][]byte
	Index     []string
}

func NewValue(
	origin string,
	role string,
	scope string,
) *Value {
	value := &Value{
		ID:     uuid.New(),
		Epoch:  Epoch.Load(),
		Tick:   Tick.Load(),
		SeqIdx: SeqIdx.Add(1),
		Origin: origin,
		Role:   role,
		Scope:  scope,
	}

	value.FormatPrefix()
	return value
}

/*
FormatPrefix recomputes and sets Prefix from current Origin, Role, Scope, Tick, SeqIdx.
*/
func (value *Value) FormatPrefix() string {
	value.Prefix = fmt.Sprintf(
		"%s/%s/%s/%019d/%019d.bin",
		value.Origin, value.Role, value.Scope, value.Tick, value.SeqIdx,
	)
	return value.Prefix
}

/*
New returns a zeroed Value ready for deserialization.
*/
func New() *Value {
	return &Value{}
}

func (value *Value) Clone() *Value {
	if value == nil {
		return nil
	}
	cp := *value
	cp.Layers = make([][]byte, len(value.Layers))
	for i, l := range value.Layers {
		cp.Layers[i] = append([]byte(nil), l...)
	}
	cp.Index = append([]string(nil), value.Index...)
	return &cp
}
