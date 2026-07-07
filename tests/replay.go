package tests

import (
	"bytes"
	"iter"
)

/*
ReplayFixture serves a recorded jsonl slice of raw Kraken frames as a Fixture,
so a real historical capture (one frame per line) drives the exact same pipeline
as the synthetic fixtures, and composes with the scenario operators for
controllable-but-realistic conditions.
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
