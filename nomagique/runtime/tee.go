package runtime

import (
	"io"
	"unsafe"

	"github.com/theapemachine/symm/data"
)

/*
Tee unites an off-ramp pusher with a consumer receiver.
*/
type Tee interface {
	io.Closer
	Push(*data.Value)
	Next() unsafe.Pointer
	Run()
	Pending() int64
}
