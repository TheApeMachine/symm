package correlation

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

func TestScores(t *testing.T) {
	Convey("Given four synchronous paths with known return direction and energy", t, func() {
		section := NewSection()
		start := time.Unix(1_700_005_500, 0).UTC()
		rows := make([]kraken.TickerData, 0, 12)

		for index, prices := range [][]float64{
			{1, 2, 4},
			{1, 4, 16},
			{4, 2, 1},
			{1, 2, 1},
		} {
			symbol := []string{"A/USD", "B/USD", "C/USD", "D/USD"}[index]

			for sampleIndex, price := range prices {
				rows = append(rows, kraken.TickerData{
					Symbol: symbol,
					Last:   decimal.NewFromFloat64(price),
					Timestamp: start.Add(
						time.Duration(sampleIndex) * time.Second,
					),
				})
			}
		}

		scores, err := section.Measure(rows)

		Convey("It should produce the exact support-weighted cohort decomposition", func() {
			So(err, ShouldBeNil)
			So(scores, ShouldHaveLength, 4)
			unitRoundoff := 1 - math.Nextafter(1, 0)
			expected := map[string]struct {
				correlation    float64
				signed         float64
				relativeEnergy float64
			}{
				"A/USD": {2.0 / 3.0, 0, 3.0 / 4.0},
				"B/USD": {2.0 / 3.0, 0, 2},
				"C/USD": {2.0 / 3.0, -2.0 / 3.0, 3.0 / 4.0},
				"D/USD": {0, 0, 3.0 / 4.0},
			}

			for symbol, decomposition := range expected {
				score := scores[symbol]
				So(score, ShouldHaveLength, 7)
				So(math.Abs(score["correlation"]-decomposition.correlation),
					ShouldBeLessThanOrEqualTo, unitRoundoff)
				So(math.Abs(score["signed"]-decomposition.signed),
					ShouldBeLessThanOrEqualTo, unitRoundoff)
				So(math.Abs(score["relativeEnergy"]-decomposition.relativeEnergy),
					ShouldBeLessThanOrEqualTo, unitRoundoff)
				excess := math.Max(0, score["relativeEnergy"]-1)
				deficit := math.Max(0, 1-score["relativeEnergy"])
				So(score["herdScore"], ShouldEqual,
					math.Max(0, score["signed"])/(1+excess))
				So(score["alphaScore"], ShouldEqual,
					(excess/(1+excess))/(1+math.Max(0, score["signed"])))
				So(score["noiseScore"], ShouldEqual,
					math.Max(0, 1-score["correlation"])/(1+excess+deficit))
				So(score["stressScore"], ShouldEqual,
					math.Max(0, -score["signed"]))
			}
		})
	})
}
