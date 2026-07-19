package tests

import (
	"iter"
)

/*
Handlers maps Kraken channel names to production On callbacks.
*/
type Handlers map[string]func([]byte)

/*
Replay drives registered handlers from a frame sequence.
*/
func Replay(handlers Handlers, frames iter.Seq[Frame]) {
	for frame := range frames {
		handler, ok := handlers[frame.Channel]

		if !ok {
			continue
		}

		handler(frame.Payload)
	}
}
