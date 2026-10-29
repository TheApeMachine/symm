package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/store"
)

func litRegion(key uint8, brightness float64) *store.Region {
	return &store.Region{Key: key, Brightness: brightness}
}

func TestSequencerState(t *testing.T) {
	Convey("Sequencer state and Step", t, func() {
		Convey("State survives Restore after ticks", func() {
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

		Convey("empty lit emits silence", func() {
			step := NewSequencer().Step(nil, false)
			So(step.Token.String(), ShouldBeEmpty)
			So(step.Silence, ShouldEqual, StepSilenceEmpty)
		})

		Convey("flat band picks brightest regime leader", func() {
			step := NewSequencer().Step([]*store.Region{
				litRegion(6, 0.2),
				litRegion(7, 0.9),
				litRegion(8, 0.1),
				litRegion(0, 0.4),
				litRegion(1, 0.3),
				litRegion(2, 0.2),
				litRegion(4, 4),
			}, false)
			So(step.Token.String(), ShouldEqual, "R07.R00")
		})

		Convey("hold band picks brightest regime leader", func() {
			step := NewSequencer().Step([]*store.Region{
				litRegion(6, 0.05),
				litRegion(0, 5),
				litRegion(8, 0.4),
				litRegion(9, 0.2),
				litRegion(3, 0.1),
				litRegion(4, 0.8),
				litRegion(5, 0.3),
			}, true)
			So(step.Token.String(), ShouldEqual, "R08.R04")
		})

		Convey("dark posture stays silent", func() {
			step := NewSequencer().Step([]*store.Region{
				litRegion(7, 0.8),
				litRegion(0, 0),
			}, false)
			So(step.Token.String(), ShouldBeEmpty)
			So(step.Silence, ShouldEqual, StepSilenceEmpty)
		})

		Convey("brightness tie keeps lower regime key", func() {
			step := NewSequencer().Step([]*store.Region{
				litRegion(8, 0.5),
				litRegion(6, 0.5),
				litRegion(2, 0.4),
				litRegion(0, 0.4),
			}, false)
			So(step.Token.String(), ShouldEqual, "R06.R00")
		})

		Convey("risk regions veto emission", func() {
			step := NewSequencer().Step([]*store.Region{
				litRegion(6, 0.6),
				litRegion(1, 0.4),
				litRegion(12, 0.2),
				litRegion(13, 0.5),
			}, false)
			So(step.Token.String(), ShouldBeEmpty)
			So(step.Silence, ShouldEqual, StepSilenceVeto)
		})

		Convey("risk at posture does not veto", func() {
			step := NewSequencer().Step([]*store.Region{
				litRegion(6, 0.6),
				litRegion(1, 0.5),
				litRegion(14, 0.5),
			}, false)
			So(step.Token.String(), ShouldEqual, "R06.R01")
		})

		Convey("sequential duplicate lit is silent", func() {
			sequencer := NewSequencer()
			lit := []*store.Region{
				litRegion(6, 0.4),
				litRegion(0, 0.3),
			}
			first := sequencer.Step(lit, false)
			second := sequencer.Step(lit, false)
			So(first.Token.String(), ShouldEqual, "R06.R00")
			So(second.Token.String(), ShouldBeEmpty)
			So(second.Silence, ShouldEqual, StepSilenceDuplicate)
		})

		Convey("emits again after leader changes", func() {
			sequencer := NewSequencer()
			_ = sequencer.Step([]*store.Region{litRegion(6, 0.4), litRegion(0, 0.3)}, false)
			next := sequencer.Step([]*store.Region{litRegion(7, 0.5), litRegion(1, 0.2)}, false)
			So(next.Token.String(), ShouldEqual, "R07.R01")
		})

		Convey("labelCode maps R10 and R11 regime/posture nibbles", func() {
			const (
				codeR10R00 = byte(0xA0) // regime 10, posture 0
				codeR11R05 = byte(0xB5) // regime 11, posture 5
			)
			So(labelCode("R10.R00"), ShouldEqual, codeR10R00)
			So(labelCode("R11.R05"), ShouldEqual, codeR11R05)
		})

		Convey("flat macro recurrence regime R10", func() {
			step := NewSequencer().Step([]*store.Region{
				litRegion(6, 0.1),
				litRegion(10, 0.85),
				litRegion(0, 0.5),
				litRegion(1, 0.2),
			}, false)
			So(step.Token.String(), ShouldEqual, "R10.R00")
		})

		Convey("hold sentiment recurrence regime R11", func() {
			step := NewSequencer().Step([]*store.Region{
				litRegion(11, 0.7),
				litRegion(8, 0.2),
				litRegion(5, 0.6),
				litRegion(4, 0.1),
			}, true)
			So(step.Token.String(), ShouldEqual, "R11.R05")
		})

		Convey("hold spell compresses oscillation via sequitur", func() {
			sequencer := NewSequencer()
			a := []*store.Region{litRegion(7, 0.5), litRegion(3, 0.4)}
			b := []*store.Region{litRegion(8, 0.5), litRegion(3, 0.4)}

			var last []string
			for range 6 {
				last = sequencer.Step(a, true).Spell
				last = sequencer.Step(b, true).Spell
			}
			So(len(last), ShouldBeLessThanOrEqualTo, 4)
		})
	})
}
