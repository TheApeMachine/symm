package trader

import "fmt"

type level3Frame struct {
	sequence uint64
	stream   string
	raw      []byte
}

/*
Level3Frames restores the atomic observation order when independent websocket
producers publish claimed sequences to the MPMC ring out of physical order.
*/
type Level3Frames struct {
	next    uint64
	pending map[uint64]level3Frame
}

/*
NewLevel3Frames starts observation order at the first claimed sequence.
*/
func NewLevel3Frames() *Level3Frames {
	return &Level3Frames{
		next:    1,
		pending: map[uint64]level3Frame{},
	}
}

/*
Add retains one observed frame until every earlier sequence is available.
*/
func (frames *Level3Frames) Add(frame level3Frame) error {
	if frame.sequence == 0 || frame.stream == "" || len(frame.raw) == 0 {
		return fmt.Errorf("trader: observed market frame is incomplete")
	}

	if frame.sequence < frames.next {
		return fmt.Errorf("trader: observed market frame sequence regressed")
	}

	if _, duplicate := frames.pending[frame.sequence]; duplicate {
		return fmt.Errorf("trader: duplicate observed market frame sequence")
	}

	frames.pending[frame.sequence] = frame
	return nil
}

/*
Next returns the next contiguous frame in observation order.
*/
func (frames *Level3Frames) Next() (level3Frame, bool) {
	frame, ok := frames.pending[frames.next]

	if !ok {
		return level3Frame{}, false
	}

	delete(frames.pending, frames.next)
	frames.next++
	return frame, true
}
