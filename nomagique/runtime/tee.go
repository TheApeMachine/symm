package runtime

import (
	"io"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
Tee unites an off-ramp pusher with a consumer receiver.
*/
type Tee interface {
	io.Closer
	Push(*data.Measurement)
	Next() unsafe.Pointer
	Pending() int
}
