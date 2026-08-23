package runtime

import (
	"sync"
	"time"
)

// SplitMix64 style random pseudo number generator
type splitMix64 struct {
	state uint64
}

func (sm64 *splitMix64) Init(seed int64) {
	sm64.state = uint64(seed)
}

func (sm64 *splitMix64) Uint64() uint64 {
	sm64.state = sm64.state + uint64(0x9E3779B97F4A7C15)

	z := sm64.state
	z = (z ^ (z >> 30)) * uint64(0xBF58476D1CE4E5B9)
	z = (z ^ (z >> 27)) * uint64(0x94D049BB133111EB)

	return z ^ (z >> 31)
}

func (sm64 *splitMix64) Int63() int64 {
	return int64(sm64.Uint64() & (1<<63 - 1))
}

var splitMix64Pool = sync.Pool{
	New: func() any {
		sm64 := &splitMix64{}
		sm64.Init(time.Now().UnixNano())
		return sm64
	},
}

func randInt() (r int) {
	sm64 := splitMix64Pool.Get().(*splitMix64)
	r = int(sm64.Int63())

	splitMix64Pool.Put(sm64)
	return
}
