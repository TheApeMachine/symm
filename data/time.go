package data

import (
	"sync/atomic"
	"time"
)

var (
	Epoch  atomic.Uint64
	Tick   atomic.Uint64
	SeqIdx atomic.Uint64
)

func init() {
	Epoch.Store(uint64(time.Now().UnixNano()))
}
