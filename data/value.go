package data

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
)

type Value struct {
	ID        [16]byte
	Err       error
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

	mu sync.Mutex
}

func NewValue(
	prefixes ...string,
) *Value {
	if len(prefixes) < 3 {
		return &Value{
			Err: errnie.Err(
				errnie.Validation,
				"[data.Value] too few prefixes",
				nil,
			),
		}
	}

	value := &Value{
		ID:        uuid.New(),
		Prefix:    strings.Join(prefixes, "/"),
		Epoch:     Epoch.Load(),
		Tick:      Tick.Load(),
		SeqIdx:    SeqIdx.Add(1),
		Timestamp: uint64(time.Now().UnixNano()),
		Origin:    prefixes[0],
		Role:      prefixes[1],
		Scope:     prefixes[2],
		Layers:    make([][]byte, 0),
	}

	return value
}

/*
New returns a zeroed Value ready for deserialization.
*/
func New() *Value {
	return &Value{}
}

func (value *Value) appendLayer(layer []byte, index string) {
	value.mu.Lock()
	value.Layers = append(value.Layers, layer)
	value.Index = append(value.Index, index)
	value.mu.Unlock()
}

func (value *Value) filled() ([][]byte, []string) {
	value.mu.Lock()
	defer value.mu.Unlock()

	return slices.Clone(value.Layers), slices.Clone(value.Index)
}
