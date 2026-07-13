package tests

import (
	"bytes"
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

/*
ReplayFixture serves a recorded jsonl slice of raw Kraken frames as a Fixture,
so a real historical capture drives the same pipeline as synthetic fixtures.
*/
type ReplayFixture struct {
	frames [][]byte
}

/*
NewReplayFixture parses newline-delimited frame payloads into a replayable
sequence, skipping blank lines.
*/
func NewReplayFixture(raw []byte) *ReplayFixture {
	lines := bytes.Split(raw, []byte("\n"))
	frames := make([][]byte, 0, len(lines))

	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)

		if len(trimmed) == 0 {
			continue
		}

		frames = append(frames, append([]byte(nil), trimmed...))
	}

	return &ReplayFixture{frames: frames}
}

func (fixture *ReplayFixture) Generate() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for _, frame := range fixture.frames {
			if !yield(frame) {
				return
			}
		}
	}
}

func (fixture *ReplayFixture) Frames() iter.Seq[Frame] {
	return FrameSequence(fixture.Generate())
}
