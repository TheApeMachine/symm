package correlation_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	nmcorrelation "github.com/theapemachine/symm/nomagique/statistic/correlation"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestPairsNext(t *testing.T) {
	Convey("Pairs evaluates pairwise dependence and Fisher significance across retained symbols", t, func() {
		pairs := nmcorrelation.NewPairs(algo.NewHayashiYoshida())

		// First symbol: BTC, 5 observations
		for index := range 5 {
			obs := nmcorrelation.PriceObservation{
				Symbol: "BTC/USD",
				Price: temporal.Price{
					At:    time.Unix(1_700_000_000+int64(index)+1, 0).UnixNano(),
					Value: 100.0 + float64(index),
				},
			}

			readings := tests.CollectSeq[nmcorrelation.PairsReading](
				pairs.Next(sequence.NewValues(obs).Next(nil)),
			)

			So(pairs.Error(), ShouldBeNil)
			So(len(readings), ShouldEqual, 1)
			So(readings[0].Path.Count, ShouldEqual, float64(index+1))
			So(readings[0].Path.Accepted, ShouldBeTrue)
			So(len(readings[0].Peers), ShouldEqual, 0)
		}

		// Second symbol: ETH, 5 observations
		var lastReading nmcorrelation.PairsReading
		for index := range 5 {
			obs := nmcorrelation.PriceObservation{
				Symbol: "ETH/USD",
				Price: temporal.Price{
					At:    time.Unix(1_700_000_000+int64(index)+1, 0).UnixNano(),
					Value: 200.0 + float64(index)*2.0,
				},
			}

			readings := tests.CollectSeq[nmcorrelation.PairsReading](
				pairs.Next(sequence.NewValues(obs).Next(nil)),
			)

			So(pairs.Error(), ShouldBeNil)
			So(len(readings), ShouldEqual, 1)
			lastReading = readings[0]
		}

		So(lastReading.Path.Count, ShouldEqual, 5.0)
		So(lastReading.PeerSymbol, ShouldEqual, "BTC/USD")
		So(len(lastReading.Peers), ShouldEqual, 1)
		So(lastReading.Selected.Defined, ShouldBeTrue)
		So(lastReading.Selected.Correlation, ShouldBeGreaterThan, 0.0)
		So(lastReading.Selected.Support, ShouldEqual, 4.0)
	})
}
