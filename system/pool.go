package system

import (
	"runtime"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/theapemachine/errnie"
)

var Q *Pool

func init() {
	Q = NewPool()
}

type Pool struct {
	handle *ants.Pool
}

func NewPool() *Pool {
	handle, err := ants.NewPool(
		runtime.GOMAXPROCS(0),
		ants.WithNonblocking(true),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[system.pool] new pool error", err,
		))
	}

	return &Pool{
		handle: handle,
	}
}

func (pool *Pool) Submit(task func()) error {
	return pool.handle.Submit(task)
}

/*
Release waits for in-flight work then closes the pool.
*/
func (pool *Pool) Release() error {
	if pool.handle == nil {
		return nil
	}

	if err := pool.handle.ReleaseTimeout(10 * time.Second); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "[system.pool] release timeout", err,
		))
	}

	return nil
}
