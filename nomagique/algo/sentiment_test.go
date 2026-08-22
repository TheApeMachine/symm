package algo

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestCohortSentiment(t *testing.T) {
	Convey("Given a cohort where every symbol advances", t, func() {
		number := nomagique.NewNumber[string](temporal.Path)
		start := time.Unix(1_700_000_000, 0).UTC()

		for _, item := range []struct {
			symbol string
			p1     float64
			p2     float64
		}{
			{"AAA/USD", 100.0, 102.0},
			{"BBB/USD", 100.0, 101.0},
			{"CCC/USD", 100.0, 101.0},
		} {
			input1 := nomagique.Frame{}
			input1.Put(nomagique.SampleValue, item.p1)
			input1.Put(nmtypes.EventTimeSec, float64(start.Unix()))
			input1.Put(nmtypes.EventTimeNsec, float64(start.Nanosecond()))
			_, err := number.Step(item.symbol, input1)
			So(err, ShouldBeNil)

			next := start.Add(time.Second)
			input2 := nomagique.Frame{}
			input2.Put(nomagique.SampleValue, item.p2)
			input2.Put(nmtypes.EventTimeSec, float64(next.Unix()))
			input2.Put(nmtypes.EventTimeNsec, float64(next.Nanosecond()))
			_, err = number.Step(item.symbol, input2)
			So(err, ShouldBeNil)
		}

		output, measured, err := CohortSentiment("AAA/USD", number)

		Convey("Breadth should be full agreement and AAA should be the leader", func() {
			So(err, ShouldBeNil)
			So(measured, ShouldBeTrue)
			So(output.MustGet(SymbolBreadth), ShouldEqual, 1.0)
			So(output.MustGet(SymbolLeaderStrength), ShouldBeGreaterThan, 0)
			So(output.MustGet(SymbolSurgeScore), ShouldBeGreaterThan, 0)
			So(output.MustGet(SymbolSlumpScore), ShouldEqual, 0)
			So(output.MustGet(SymbolDivergentScore), ShouldEqual, 0)
			So(output.MustGet(SymbolSentimentStrength), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a leader moving against the rest of the cohort", t, func() {
		number := nomagique.NewNumber[string](temporal.Path)
		start := time.Unix(1_700_000_000, 0).UTC()

		for _, item := range []struct {
			symbol string
			p1     float64
			p2     float64
		}{
			{"AAA/USD", 100.0, 108.0},
			{"BBB/USD", 100.0, 99.0},
			{"CCC/USD", 100.0, 99.0},
		} {
			input1 := nomagique.Frame{}
			input1.Put(nomagique.SampleValue, item.p1)
			input1.Put(nmtypes.EventTimeSec, float64(start.Unix()))
			input1.Put(nmtypes.EventTimeNsec, float64(start.Nanosecond()))
			_, err := number.Step(item.symbol, input1)
			So(err, ShouldBeNil)

			next := start.Add(time.Second)
			input2 := nomagique.Frame{}
			input2.Put(nomagique.SampleValue, item.p2)
			input2.Put(nmtypes.EventTimeSec, float64(next.Unix()))
			input2.Put(nmtypes.EventTimeNsec, float64(next.Nanosecond()))
			_, err = number.Step(item.symbol, input2)
			So(err, ShouldBeNil)
		}

		output, measured, err := CohortSentiment("AAA/USD", number)

		Convey("Divergence should fire for the evidenced leader", func() {
			So(err, ShouldBeNil)
			So(measured, ShouldBeTrue)
			So(output.MustGet(SymbolDivergentScore), ShouldBeGreaterThan, 0)
			So(output.MustGet(SymbolSlumpScore), ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkCohortSentiment(b *testing.B) {
	const cohortSize = 200
	number := nomagique.NewNumber[string](temporal.Path)
	start := time.Unix(1_700_000_000, 0).UTC()

	for index := range cohortSize {
		symbol := string(rune('A'+index%26)) + "/USD"
		input1 := nomagique.Frame{}
		input1.Put(nomagique.SampleValue, 100.0)
		input1.Put(nmtypes.EventTimeSec, float64(start.Unix()))
		input1.Put(nmtypes.EventTimeNsec, float64(start.Nanosecond()))
		_, _ = number.Step(symbol, input1)

		input2 := nomagique.Frame{}
		input2.Put(nomagique.SampleValue, 101.0+float64(index)*0.01)
		input2.Put(nmtypes.EventTimeSec, float64(start.Add(time.Second).Unix()))
		input2.Put(nmtypes.EventTimeNsec, float64(start.Add(time.Second).Nanosecond()))
		_, _ = number.Step(symbol, input2)
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _, _ = CohortSentiment("A/USD", number)
	}
}
