package strategy

import (
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestThesisEvidence(t *testing.T) {
	Convey("Given a thesis with repeated evidence from one source", t, func() {
		thesis := NewThesis()
		thesis.AddEvidence("BTC/USD", "manifold", 1)
		thesis.AddEvidence("BTC/USD", "manifold", 2)

		Convey("When the current evidence is requested", func() {
			value, ok := thesis.Evidence("BTC/USD", "manifold")

			Convey("Then the newest snapshot replaces the active source value", func() {
				So(ok, ShouldBeTrue)
				So(value, ShouldEqual, 2)
				values, err := thesis.Values("BTC/USD")
				So(err, ShouldBeNil)
				So(values, ShouldHaveLength, 1)
			})
		})
	})
}

func TestThesisSymbols(t *testing.T) {
	Convey("Given compatibility evidence and a typed-only logic epoch", t, func() {
		thesis := NewThesis()
		thesis.AddEvidence("Z/USD", "ticker", 1)
		epoch := epochJournalValue(time.Unix(1, 0), 1)
		epoch.Symbol = "A/USD"
		epoch.Measurements[0].Symbol = "A/USD"
		So(thesis.RecordEpochs([]types.LogicEpoch{epoch}), ShouldBeNil)

		Convey("When active symbols are requested", func() {
			Convey("Then both migration paths remain visible and deterministic", func() {
				So(thesis.Symbols(), ShouldResemble, []string{"A/USD", "Z/USD"})
			})
		})
	})
}

func TestThesisAddEvidence(t *testing.T) {
	Convey("Given concurrent signal and logic producers", t, func() {
		thesis := NewThesis()
		wait := sync.WaitGroup{}
		wait.Add(32)

		Convey("When they append evidence to one symbol", func() {
			for index := range 32 {
				go func() {
					defer wait.Done()
					thesis.AddEvidence("BTC/USD", fmt.Sprintf("source-%d", index), index)
				}()
			}

			wait.Wait()

			Convey("Then every independent source is retained", func() {
				values, err := thesis.Values("BTC/USD")
				So(err, ShouldBeNil)
				So(values, ShouldHaveLength, 32)
			})
		})
	})
}

func BenchmarkThesisAddEvidence(b *testing.B) {
	thesis := NewThesis()

	for b.Loop() {
		thesis.AddEvidence("BTC/USD", "ticker", 1.0)
	}
}
