package strategy

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestCognitionAdjustment(t *testing.T) {
	Convey("Given ready cognition with a learned continuation", t, func() {
		conviction := 0.02
		confidence := 0.8
		probability := 0.25
		cognition := types.Cognition{
			Ready:          true,
			Confidence:     confidence,
			LookaheadPaths: 1,
			LookaheadScore: math.Log(probability),
		}

		Convey("It should discount conviction by continuation probability without adding direction", func() {
			adjustment := cognitionAdjustment(conviction, cognition)
			expected := conviction * math.Expm1(math.Log(probability)) * confidence

			So(adjustment, ShouldAlmostEqual, expected)
			So(adjustment, ShouldBeLessThanOrEqualTo, 0.0)
		})

		Convey("It should leave conviction unchanged when the continuation is certain", func() {
			cognition.LookaheadScore = 0

			So(cognitionAdjustment(conviction, cognition), ShouldEqual, 0.0)
		})

		Convey("It should reject a positive score because log probabilities cannot be positive", func() {
			cognition.LookaheadScore = 0.1

			So(cognitionAdjustment(conviction, cognition), ShouldEqual, 0.0)
		})
	})

	Convey("Given cognition that is not ready", t, func() {
		cognition := types.Cognition{
			Confidence:     1,
			LookaheadPaths: 1,
			LookaheadScore: math.Log(0.25),
		}

		Convey("It should not alter the forecast", func() {
			So(cognitionAdjustment(0.02, cognition), ShouldEqual, 0.0)
		})
	})
}

func TestGetCausalHistoryRows(t *testing.T) {
	Convey("Given a causal output without aligned history", t, func() {
		thesis := types.NewThesis()
		thesis.Causal.Store("BTC/USD", map[string]any{
			"intervention":  0.2,
			"doExpectation": 0.1,
		})

		Convey("It should not fabricate a causal row from unrelated output fields", func() {
			So(getCausalHistoryRows(thesis, "BTC/USD"), ShouldBeNil)
		})
	})

	Convey("Given aligned causal history", t, func() {
		thesis := types.NewThesis()
		rows := [][]float64{{0.1, 0.2, 0.3, 0.4}}
		thesis.Causal.Store("BTC/USD", map[string]any{"historyRows": rows})

		Convey("It should return the real aligned rows", func() {
			So(getCausalHistoryRows(thesis, "BTC/USD"), ShouldResemble, rows)
		})
	})
}
