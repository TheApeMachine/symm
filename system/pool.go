package system

import (
	"runtime"

	"github.com/panjf2000/ants/v2"
)

var Q *Pool

func init() {
	Q = NewPool()
}

type Pool struct {
	handle *ants.Pool
}

func NewPool() *Pool {
	handle, err := ants.NewPool(runtime.GOMAXPROCS(0))

	if err != nil {
		panic(err)
	}

	return &Pool{
		handle: handle,
	}
}

func (pool *Pool) Submit(task func()) error {
	return pool.handle.Submit(task)
}
