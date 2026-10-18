package transport

import (
	"io"
	"iter"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
)

/*
Next yields each Value from reader. A stream reports EOF at the end of one
value and can produce the next on the following read, so EOF with no bytes
is the end of the run. ReadFrom is used directly so a source that implements
WriterTo is consumed one value at a time.
*/
func Next(reader io.Reader) iter.Seq[*data.Value] {
	return func(yield func(*data.Value) bool) {
		for {
			value := data.New()
			_, err := value.ReadFrom(reader)

			if err == io.EOF {
				return
			}

			if err != nil {
				value.Err = errnie.Error(errnie.Err(
					errnie.IO,
					"[transport] failed to read value",
					err,
				))

				yield(value)
				return
			}

			if !yield(value) {
				return
			}
		}
	}
}
