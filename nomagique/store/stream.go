package store

/*
Stream measures channel deformation within one symbol's own tape. Grid cells
are keyed by metric label alone, so one cell is observed by every symbol;
deforming a channel against the previous value of another symbol would turn
symbol interleaving into false co-movement. Each tape (a live symbol, or one
replayed fragment) owns its own Stream.

A Stream is not safe for concurrent use: it belongs to the single consumer
of its tape.
*/
type Stream struct {
	previous map[string]float64
}

/*
NewStream returns a Stream that has observed nothing.
*/
func NewStream() *Stream {
	return &Stream{previous: make(map[string]float64)}
}

/*
Deform folds one pass of raw channel values into the stream and answers the
deformation of every channel the stream observed before. A channel seen for
the first time has no movement yet and answers nothing.
*/
func (stream *Stream) Deform(channels map[string]float64) map[string]float64 {
	deformations := make(map[string]float64, len(channels))

	for channel, raw := range channels {
		if previous, ok := stream.previous[channel]; ok {
			deformations[channel] = deform(previous, raw)
		}

		stream.previous[channel] = raw
	}

	return deformations
}
