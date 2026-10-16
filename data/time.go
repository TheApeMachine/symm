package data

import "sync/atomic"

var (
	Epoch  atomic.Uint64
	Tick   atomic.Uint64
	SeqIdx atomic.Uint64
)
