package tokenizer

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/store"
	. "github.com/smartystreets/goconvey/convey"
)

func TestSequencerStateRoundTrip(t *testing.T) {
	Convey("Sequencer State survives Restore after ticks", t, func() {
		sequencer := NewSequencer()
		lit := []*store.Region{
			{Key: 6, Brightness: 0.4},
			{Key: 0, Brightness: 0.3},
		}
		first := sequencer.Step(lit, false)
		So(first.Token.String(), ShouldNotBeBlank)

		saved := sequencer.State()
		restored := NewSequencer()
		restored.Restore(saved)

		second := restored.Step([]*store.Region{
			{Key: 7, Brightness: 0.5},
			{Key: 1, Brightness: 0.2},
		}, false)
		So(second.Token.String(), ShouldNotBeBlank)
		So(len(restored.State().EnterRoot), ShouldBeGreaterThan, 0)
	})
}
