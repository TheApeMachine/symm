package depthflow

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestNewSignal(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	Convey("Given a signal fed one Level3 envelope", t, func() {
		signal := NewSignal(context.Background())

		// A real subscription opens with a snapshot: the resident book
		// refuses to report depth from an increment alone, since that
		// describes a book the process never saw.
		envelope := types.NewEnvelope(types.EnvelopeLevel3)
		envelope.Level3Data = level3Message("BTC/USD", now,
			[]kraken.Level3Order{level3Order("add", 99, 2, now)},
			[]kraken.Level3Order{level3Order("add", 101, 2, now)},
		)
		envelope.Level3Data.Type = "snapshot"

		Convey("Name reports the signal identity", func() {
			So(signal.Name(), ShouldEqual, "depthflow")
		})

		Convey("Error is nil on a healthy signal", func() {
			So(signal.Error(), ShouldBeNil)
		})

		Convey("Step delegates to the Level3 entity and writes DepthFlow", func() {
			result := signal.Step(envelope)

			So(result.DepthFlow, ShouldNotBeNil)
			So(result.DepthFlow.Err, ShouldBeNil)
			So(result.DepthFlow.Metrics, ShouldNotBeEmpty)
		})

		Convey("Close releases the signal", func() {
			So(signal.Close(), ShouldBeNil)
		})
	})
}
