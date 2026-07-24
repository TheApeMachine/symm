package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	pmanifold "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

func TestBasinConfidence(t *testing.T) {
	Convey("Given a GasReady manifold without a phase corpus", t, func() {
		thesis := types.NewThesis()
		thesis.Manifold.Store("BTC/USD", gasState("BTC/USD", false, nil))

		basin, ready, class, phaseReady, _ := basinConfidence(thesis, "BTC/USD", "buy")

		Convey("It leaves the basin unready rather than inventing coherence", func() {
			So(basin, ShouldEqual, 0)
			So(ready, ShouldBeFalse)
			So(class, ShouldEqual, "")
			So(phaseReady, ShouldBeFalse)
		})
	})

	Convey("Given a phase compass aligned with the cognitive winner", t, func() {
		thesis := types.NewThesis()
		thesis.Manifold.Store("BTC/USD", gasState("BTC/USD", true, []manifold.PhaseResponse{
			{Angle: 0.1, Similarity: 0.2, Outcome: manifold.PhaseOutcome{Class: "sell", Confidence: 0.9}},
			{Angle: 1.2, Similarity: 0.8, Outcome: manifold.PhaseOutcome{Class: "buy", Confidence: 0.7}},
		}))

		basin, ready, class, phaseReady, similarity := basinConfidence(thesis, "BTC/USD", "buy")

		Convey("It takes the strongest constructive buy alignment as basin", func() {
			So(phaseReady, ShouldBeTrue)
			So(ready, ShouldBeTrue)
			So(class, ShouldEqual, "buy")
			So(basin, ShouldEqual, 0.8)
			So(similarity, ShouldEqual, 0.8)
		})
	})

	Convey("Given a phase compass whose strongest hit is an opposing class", t, func() {
		thesis := types.NewThesis()
		thesis.Manifold.Store("ETH/USD", gasState("ETH/USD", true, []manifold.PhaseResponse{
			{Angle: 0.5, Similarity: 0.9, Outcome: manifold.PhaseOutcome{Class: "sell", Confidence: 0.8}},
		}))

		basin, ready, class, phaseReady, similarity := basinConfidence(thesis, "ETH/USD", "buy")

		Convey("It reports phase ready without a confirming basin", func() {
			So(phaseReady, ShouldBeTrue)
			So(ready, ShouldBeFalse)
			So(basin, ShouldEqual, 0)
			So(class, ShouldEqual, "sell")
			So(similarity, ShouldEqual, 0.9)
		})
	})
}

func TestMeasureOpportunityPhase(t *testing.T) {
	Convey("Given destructive interference with a sell-labeled history", t, func() {
		thesis := types.NewThesis()
		thesis.Manifold.Store("BTC/USD", gasState("BTC/USD", true, []manifold.PhaseResponse{
			{Angle: 0, Similarity: -0.6, Outcome: manifold.PhaseOutcome{Class: "sell"}},
		}))

		reading := measureOpportunity(
			types.Forecasts{
				Symbol: "BTC/USD", ExpectedReturn: 0.05, Uncertainty: 0.01, HorizonEvents: 1,
			},
			types.Cognition{Winner: "buy", Confidence: 0.9, Contrast: 0.4},
			thesis,
		)

		Convey("It does not veto — antipode is not opposition", func() {
			So(reading.PhaseReady, ShouldBeTrue)
			So(reading.PhaseSimilarity, ShouldEqual, -0.6)
			So(reading.PhaseOpposes(), ShouldBeFalse)
			So(reading.BasinReady, ShouldBeFalse)
		})
	})

	Convey("Given cognition buy against an opposing phase attractor", t, func() {
		thesis := types.NewThesis()
		thesis.Manifold.Store("BTC/USD", gasState("BTC/USD", true, []manifold.PhaseResponse{
			{Angle: 0, Similarity: 0.7, Outcome: manifold.PhaseOutcome{Class: "sell"}},
		}))

		reading := measureOpportunity(
			types.Forecasts{
				Symbol: "BTC/USD", ExpectedReturn: 0.05, Uncertainty: 0.01, HorizonEvents: 1,
			},
			types.Cognition{Winner: "buy", Confidence: 0.9, Contrast: 0.4},
			thesis,
		)

		Convey("It marks phase opposition for the admit gate", func() {
			So(reading.PhaseReady, ShouldBeTrue)
			So(reading.PhaseOpposes(), ShouldBeTrue)
			So(reading.BasinReady, ShouldBeFalse)
			So(reading.Reserved(), ShouldBeFalse)
		})
	})

	Convey("Given cognition buy aligned with a constructive phase basin", t, func() {
		thesis := types.NewThesis()
		thesis.Manifold.Store("BTC/USD", gasState("BTC/USD", true, []manifold.PhaseResponse{
			{Angle: 0, Similarity: 0.4, Outcome: manifold.PhaseOutcome{Class: "buy"}},
		}))

		reading := measureOpportunity(
			types.Forecasts{
				Symbol: "BTC/USD", ExpectedReturn: 0.08, Uncertainty: 0.01, HorizonEvents: 1,
			},
			types.Cognition{
				Winner: "buy", Confidence: 0.95, Contrast: 0.5, Ambiguous: false,
			},
			thesis,
		)

		Convey("It scores lead against the phase basin for reserved overflow", func() {
			So(reading.PhaseOpposes(), ShouldBeFalse)
			So(reading.BasinReady, ShouldBeTrue)
			So(reading.Basin, ShouldEqual, 0.4)
			So(reading.Lead, ShouldAlmostEqual, 0.55)
			So(reading.Reserved(), ShouldBeTrue)
		})
	})
}

func BenchmarkBasinConfidence(b *testing.B) {
	thesis := types.NewThesis()
	thesis.Manifold.Store("BTC/USD", gasState("BTC/USD", true, []manifold.PhaseResponse{
		{Angle: 0, Similarity: 0.6, Outcome: manifold.PhaseOutcome{Class: "buy"}},
		{Angle: 1, Similarity: 0.2, Outcome: manifold.PhaseOutcome{Class: "sell"}},
	}))

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _, _, _, _ = basinConfidence(thesis, "BTC/USD", "buy")
	}
}

func gasState(
	symbol string,
	phaseReady bool,
	scan []manifold.PhaseResponse,
) manifold.State {
	return manifold.State{
		Source:         "manifold",
		Symbol:         symbol,
		At:             time.Unix(1, 0).UTC(),
		Duration:       time.Second,
		Epoch:          1,
		ReferencePrice: decimal.NewFromInt64(100),
		Spread:         0.001,
		BuyCapacity:    decimal.NewFromInt64(50),
		SellCapacity:   decimal.NewFromInt64(50),
		InvalidReason:  manifold.Valid,
		BuyIntensity:   1,
		SellIntensity:  0.5,
		SpectralRadius: 0.1,
		Reading: pmanifold.Reading{
			PressureGradX: 0.1,
			Divergence:    -0.1,
			CoherenceMag2: 0.5,
			GuidanceSpeed: 0.1,
		},
		PhaseReady: phaseReady,
		PhaseScan:  scan,
	}
}
