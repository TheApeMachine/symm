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
	Push(*data.Measurement[float64])
	Next() unsafe.Pointer
	io.Closer
}
