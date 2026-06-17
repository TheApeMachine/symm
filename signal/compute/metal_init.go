package compute

import (
	"runtime"
	"sync/atomic"
)

var metalInitGate atomic.Uint32

/*
WithMetalInit serializes Metal device, library, and pipeline creation.

Concurrent first use of the physics and learning manifold solvers can exceed the
AGX compiled-variant footprint limit on Apple Silicon.
*/
func WithMetalInit(init func() error) error {
	for !metalInitGate.CompareAndSwap(0, 1) {
		runtime.Gosched()
	}

	defer metalInitGate.Store(0)

	return init()
}
