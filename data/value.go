package data

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

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

	// Reserved slots [base, base+slots) are claimed lock-free by the nodes
	// that run in parallel on this Value (see Reserve / claim).
	base  int
	slots int
	next  atomic.Int32
}

/*
Reserve pre-sizes n empty layer slots, one per pipeline node, so parallel
nodes append without a lock: each claims its own slot via an atomic cursor.
*/
func (value *Value) Reserve(n int) *Value {
	value.base = len(value.Layers)
	value.slots = n
	value.next.Store(0)
	value.Layers = append(value.Layers, make([][]byte, n)...)
	value.Index = append(value.Index, make([]string, n)...)

	return value
}

/*
Reserved reports whether layer slots were already reserved for this Value.
*/
func (value *Value) Reserved() bool {
	return value.slots > 0
}

/*
claim stores one layer in the next reserved slot, or appends when no slot is
left (single-writer use outside the pipeline).
*/
func (value *Value) claim(layer []byte, index string) {
	if i := int(value.next.Add(1) - 1); i < value.slots {
		value.Layers[value.base+i] = layer
		value.Index[value.base+i] = index

		return
	}

	value.Layers = append(value.Layers, layer)
	value.Index = append(value.Index, index)
}

/*
visible is how many layers readers may scan: reserved slots are being
written in parallel, so they stay hidden until the Value is serialized.
*/
func (value *Value) visible() int {
	if value.slots > 0 {
		return value.base
	}

	return len(value.Layers)
}

/*
filled returns the layers and indexes without unused (nil) reserved slots.
*/
func (value *Value) filled() ([][]byte, []string) {
	layers := make([][]byte, 0, len(value.Layers))
	index := make([]string, 0, len(value.Layers))

	for i, layer := range value.Layers {
		if layer == nil {
			continue
		}

		idx := ""
		if i < len(value.Index) {
			idx = value.Index[i]
		}

		layers = append(layers, layer)
		index = append(index, idx)
	}

	return layers, index
}

func NewValue(
	origin string,
	role string,
	scope string,
) *Value {
	value := &Value{
		ID:        uuid.New(),
		Epoch:     Epoch.Load(),
		Tick:      Tick.Load(),
		SeqIdx:    SeqIdx.Add(1),
		Timestamp: uint64(time.Now().UnixNano()),
		Origin:    origin,
		Role:      role,
		Scope:     scope,
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
		value.Origin, value.Role, value.Scope, value.Epoch, value.Tick,
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
	cp := &Value{
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

	layers, index := value.filled()
	cp.Layers = make([][]byte, len(layers))
	cp.Index = index

	for i, l := range layers {
		cp.Layers[i] = append([]byte(nil), l...)
	}

	return cp
}
