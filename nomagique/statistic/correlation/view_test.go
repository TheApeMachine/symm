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

func TestLastPriceNext(t *testing.T) {
	Convey("LastPrice yields the observation price", t, func() {
		lastPrice := nmcorrelation.NewLastPrice()
		observation := nmcorrelation.PriceObservation{
			Symbol: "BTC/USD",
			Price:  temporal.Price{At: time.Unix(1_700_000_000, 0).UnixNano(), Value: 101.5},
		}
		out := tests.CollectSeq[float64](lastPrice.Next(sequence.NewValues(observation).Next(nil)))
		So(lastPrice.Error(), ShouldBeNil)
		So(out, ShouldResemble, []float64{101.5})
	})
}

func TestSignedNext(t *testing.T) {
	Convey("Signed yields defined pair correlation and stays silent otherwise", t, func() {
		pairs := nmcorrelation.NewPairs(algo.NewHayashiYoshida(), "ETH/USD", "BTC/USD")
		signed := nmcorrelation.NewSigned()

		for index := range 5 {
			obs := nmcorrelation.PriceObservation{
				Symbol: "BTC/USD",
				Price: temporal.Price{
					At:    time.Unix(1_700_000_000+int64(index)+1, 0).UnixNano(),
					Value: 100.0 + float64(index),
				},
			}
			reading := tests.CollectSeq[nmcorrelation.PairsReading](
				pairs.Next(sequence.NewValues(obs).Next(nil)),
			)
			out := tests.CollectSeq[float64](signed.Next(sequence.NewValues(reading[0]).Next(nil)))
			So(len(out), ShouldEqual, 0)
		}

		var last float64

		for index := range 5 {
			obs := nmcorrelation.PriceObservation{
				Symbol: "ETH/USD",
				Price: temporal.Price{
					At:    time.Unix(1_700_000_000+int64(index)+1, 0).UnixNano(),
					Value: 200.0 + float64(index)*2.0,
				},
			}
			reading := tests.CollectSeq[nmcorrelation.PairsReading](
				pairs.Next(sequence.NewValues(obs).Next(nil)),
			)
			out := tests.CollectSeq[float64](signed.Next(sequence.NewValues(reading[0]).Next(nil)))

			if index == 4 {
				So(len(out), ShouldEqual, 1)
				last = out[0]
			}
		}

		So(last, ShouldBeGreaterThan, 0.0)
	})
}
