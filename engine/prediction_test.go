package engine

import (
	"testing"

	"github.com/theapemachine/symm/kraken/asset"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPredictionLeadMeasurement(t *testing.T) {
	Convey("Given multiple priced perspective observations", t, func() {
		prediction := Prediction{
			Perspective: Perspective{
				Measurements: []Measurement{
					{
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.4,
						Last:       100,
					},
					{
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.9,
						Bid:        199,
						Ask:        201,
					},
				},
			},
		}

		lead, ok := prediction.LeadMeasurement()

		Convey("It should return the highest-confidence anchor", func() {
			So(ok, ShouldBeTrue)
			So(lead.Confidence, ShouldEqual, 0.9)
			So(lead.AnchorPrice(), ShouldEqual, 200)
		})
	})
}

func TestPredictionError(t *testing.T) {
	Convey("Given a due prediction and matching ground truth", t, func() {
		prediction := Prediction{
			Direction:      1,
			ExpectedReturn: 0.02,
			Perspective: Perspective{
				Measurements: []Measurement{
					{
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.9,
						Last:       100,
					},
				},
			},
		}

		forecastError, ok := prediction.Error(Measurement{
			Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
			Last:  102,
		})

		Convey("It should settle actual return and signed error", func() {
			So(ok, ShouldBeTrue)
			So(prediction.ActualReturn, ShouldAlmostEqual, 0.02, 1e-9)
			So(forecastError, ShouldAlmostEqual, 0, 1e-9)
			So(prediction.Err, ShouldAlmostEqual, 0, 1e-9)
		})
	})

	Convey("Given a short prediction and a downward move", t, func() {
		prediction := Prediction{
			Direction:      -1,
			ExpectedReturn: 0.02,
			Perspective: Perspective{
				Measurements: []Measurement{
					{
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.9,
						Last:       100,
					},
				},
			},
		}

		forecastError, ok := prediction.Error(Measurement{
			Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
			Last:  98,
		})

		Convey("It should flip sign so a down move counts as gain for shorts", func() {
			So(ok, ShouldBeTrue)
			So(prediction.ActualReturn, ShouldAlmostEqual, 0.02, 1e-9)
			So(forecastError, ShouldAlmostEqual, 0, 1e-9)
			So(prediction.Err, ShouldAlmostEqual, 0, 1e-9)
		})
	})

	Convey("Given ground truth for a different symbol", t, func() {
		prediction := Prediction{
			Perspective: Perspective{
				Measurements: []Measurement{
					{
						Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
						Confidence: 0.9,
						Last:       100,
					},
				},
			},
		}

		_, ok := prediction.Error(Measurement{
			Pairs: []asset.Pair{{Wsname: "ETH/EUR"}},
			Last:  102,
		})

		Convey("It should refuse to settle", func() {
			So(ok, ShouldBeFalse)
		})
	})
}
