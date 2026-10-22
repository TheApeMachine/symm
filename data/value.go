package data

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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
	origin, role, scope string,
) *Value {
	epoch := Epoch.Load()
	tick := Tick.Load()

	value := &Value{
		ID: uuid.New(),
		Prefix: strings.Join([]string{
			origin,
			role,
			scope,
			fmt.Sprintf("%019d", epoch),
			fmt.Sprintf("%019d", tick),
		}, "/"),
		Epoch:     epoch,
		Tick:      tick,
		SeqIdx:    SeqIdx.Add(1),
		Timestamp: uint64(time.Now().UnixNano()),
		Origin:    origin,
		Role:      role,
		Scope:     scope,
		Layers:    make([][]byte, 0),
		Index:     make([]string, 0),
	}

	return value
}

/*
New returns a zeroed Value ready for deserialization.
*/
func New() *Value {
	return &Value{}
}
