package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	symmmanifold "github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
TestOpportunity_stance proves an established active reversal vetoes a long
even when an equal number of initiative categories favor it.
*/
func TestOpportunity_stance(t *testing.T) {
	Convey("Given ignition and active reversal in the same decision cut", t, func() {
		at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		normalized := 1.0
		graph := types.NewGraph("OXT/USD")
		err := graph.Evidence.AddMeasurements([]*types.Measurement{
			{
				Source:     types.SourcePumpDump,
				Stream:     types.PumpDump,
				Metric:     types.MetricIgnition,
				Subject:    types.SubjectPumpIgnition,
				Symbol:     "OXT/USD",
				At:         at,
				Unit:       types.UnitDimensionless,
				Raw:        1,
				Normalized: &normalized,
				Validity: types.MeasurementValidity{
					State: types.ValidityValid,
				},
			},
			{
				Source:     types.SourceExhaustion,
				Stream:     types.Exhaust,
				Metric:     types.MetricReversal,
				Symbol:     "OXT/USD",
				At:         at,
				Unit:       types.UnitDimensionless,
				Raw:        1,
				Normalized: &normalized,
				Validity: types.MeasurementValidity{
					State: types.ValidityValid,
				},
			},
		})
		So(err, ShouldBeNil)
		graph.Compose()
		thesis := types.NewThesis(nil, nil)
		thesis.Graphs.Store("OXT/USD", graph)

		Convey("Then structural reversal vetoes the balanced category count", func() {
			evidence, vetoed := (&Opportunity{}).stance(thesis, "OXT/USD")

			So(evidence.Favors, ShouldContain, string(types.VerticalIgnition))
			So(evidence.Favors, ShouldContain, string(types.RiskOnSurge))
			So(evidence.Opposes, ShouldContain, string(types.ActiveReversal))
			So(evidence.Opposes, ShouldContain, string(types.FadedExhaustion))
			So(evidence.Vetoes, ShouldContain, string(types.ActiveReversal))
			So(len(evidence.Favors), ShouldEqual, len(evidence.Opposes))
			So(vetoed, ShouldBeTrue)
		})
	})
}

/*
TestOpportunityReservedNeedsMarginAndLead requires both strong economic edge
and meaningful cognitive lead; SNR alone or lead alone is not reserved.
*/
func TestOpportunityReservedNeedsMarginAndLead(t *testing.T) {
	t.Parallel()

	thesis := types.NewThesis(nil, nil)
	thesis.Manifold.Store("OXT/USD", readyBasin("OXT/USD", 0.2))

	forecast := decideForecast("OXT/USD", 0.08, 0.02)
	cognition := buyCognition("OXT/USD")
	cognition.Confidence = 0.7

	reading := measureOpportunity(forecast, cognition, thesis)

	if reading.Margin <= reading.Uncertainty ||
		reading.Lead <= reading.Noise ||
		!reading.Reserved() {
		t.Fatalf("want reserved-ready reading, got %+v", reading)
	}

	snrOnly := measureOpportunity(
		decideForecast("OXT/USD", 0.08, 0.02),
		buyCognition("OXT/USD"), // confidence 0.6, basin 2/(1+2)=0.667 → negative lead
		func() *types.Thesis {
			cut := types.NewThesis(nil, nil)
			cut.Manifold.Store("OXT/USD", readyBasin("OXT/USD", 2.0))
			return cut
		}(),
	)

	if snrOnly.Reserved() {
		t.Fatalf("margin without lead must not be reserved: %+v", snrOnly)
	}

	leadOnly := measureOpportunity(
		decideForecast("OXT/USD", 0.01, 0.05),
		cognition,
		thesis,
	)

	if leadOnly.Reserved() {
		t.Fatalf("lead without margin must not be reserved: %+v", leadOnly)
	}
}

/*
TestOpportunityReservedRejectsWeakSNR ensures Margin must exceed Uncertainty
so reserved is not tripped by a barely-positive edge.
*/
func TestOpportunityReservedRejectsWeakSNR(t *testing.T) {
	t.Parallel()

	thesis := types.NewThesis(nil, nil)
	thesis.Manifold.Store("OXT/USD", readyBasin("OXT/USD", 0.2))

	// Margin = 0.01, Uncertainty = 0.02 → fails Margin > Uncertainty.
	reading := measureOpportunity(
		decideForecast("OXT/USD", 0.03, 0.02),
		buyCognition("OXT/USD"),
		thesis,
	)

	if reading.Reserved() {
		t.Fatalf("weak SNR must not be reserved: %+v", reading)
	}
}

/*
TestOpportunityReservedRejectsAmbiguous keeps overflow closed when the entropy
gate marks cognition ambiguous even if margin and lead look strong.
*/
func TestOpportunityReservedRejectsAmbiguous(t *testing.T) {
	t.Parallel()

	thesis := types.NewThesis(nil, nil)
	thesis.Manifold.Store("OXT/USD", readyBasin("OXT/USD", 0.2))

	cognition := buyCognition("OXT/USD")
	cognition.Confidence = 0.7
	cognition.Ambiguous = true

	reading := measureOpportunity(
		decideForecast("OXT/USD", 0.08, 0.02),
		cognition,
		thesis,
	)

	if reading.Reserved() {
		t.Fatalf("ambiguous cognition must not be reserved: %+v", reading)
	}
}

/*
TestOpportunityReservedRejectsZeroContrast requires winner separation before
overflow is allowed.
*/
func TestOpportunityReservedRejectsZeroContrast(t *testing.T) {
	t.Parallel()

	thesis := types.NewThesis(nil, nil)
	thesis.Manifold.Store("OXT/USD", readyBasin("OXT/USD", 0.2))

	cognition := buyCognition("OXT/USD")
	cognition.Confidence = 0.7
	cognition.Contrast = 0

	reading := measureOpportunity(
		decideForecast("OXT/USD", 0.08, 0.02),
		cognition,
		thesis,
	)

	if reading.Reserved() {
		t.Fatalf("zero contrast must not be reserved: %+v", reading)
	}
}

/*
TestOpportunityLeadNeutralWithoutManifold keeps CognitiveLead at zero when the
manifold basin is unavailable instead of inventing dynamics.
*/
func TestOpportunityLeadNeutralWithoutManifold(t *testing.T) {
	t.Parallel()

	reading := measureOpportunity(
		decideForecast("OXT/USD", 0.08, 0.02),
		buyCognition("OXT/USD"),
		types.NewThesis(nil, nil),
	)

	if reading.BasinReady || reading.Lead != 0 || reading.Reserved() {
		t.Fatalf("missing manifold must neutralize lead: %+v", reading)
	}
}

/*
TestOpportunityBasinMatchesConfidenceScale maps physical coherence onto [0,1)
so Lead is a difference of commensurate quantities.
*/
func TestOpportunityBasinMatchesConfidenceScale(t *testing.T) {
	t.Parallel()

	thesis := types.NewThesis(nil, nil)
	thesis.Manifold.Store("OXT/USD", readyBasin("OXT/USD", 3))
	reading := measureOpportunity(
		decideForecast("OXT/USD", 0.08, 0.02),
		buyCognition("OXT/USD"),
		thesis,
	)

	want := 3.0 / 4.0

	if reading.Basin != want {
		t.Fatalf("want basin %v, got %v", want, reading.Basin)
	}

	if reading.Basin >= 1 || reading.Basin <= 0 {
		t.Fatalf("basin must sit in (0,1), got %v", reading.Basin)
	}
}

func readyBasin(symbol string, coherence float64) symmmanifold.State {
	return symmmanifold.State{
		Source:         "manifold",
		Symbol:         symbol,
		At:             time.Unix(1, 0).UTC(),
		Duration:       time.Second,
		Epoch:          1,
		ReferencePrice: decimal.NewFromInt64(1),
		Spread:         0.001,
		BuyCapacity:    decimal.NewFromInt64(1000),
		SellCapacity:   decimal.NewFromInt64(1000),
		InvalidReason:  symmmanifold.Valid,
		BuyIntensity:   1,
		SellIntensity:  0.5,
		SpectralRadius: 0.1,
		Reading: fluid.Reading{
			PressureGradX: 0.1,
			Divergence:    -0.1,
			CoherenceMag2: coherence,
			GuidanceSpeed: 0.1,
		},
	}
}

/*
TestOpportunityFrictionPricesTouchConsumption proves ExpectedImpact is the
observed spread scaled by the share of the visible ask touch the planned
max_fraction entry consumes, and that the full-touch upper bound holds when
the plan would consume the whole touch or wallet cash is unknown.
*/
func TestOpportunityFrictionPricesTouchConsumption(t *testing.T) {
	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	t.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })
	viper.Set("market.quote_currency", "USD")

	price := broker.NewPrice(nil)
	_ = price.RememberFee("LRC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})

	funded := broker.NewBalance(nil, nil, nil)
	funded.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"5000","available":"5000","reserved":"0"}]}`,
	))

	forecast := func(buyCapacity int64) *types.Forecasts {
		return &types.Forecasts{
			Symbol:         "LRC/USD",
			ExpectedSpread: 0.004,
			BuyCapacity:    decimal.NewFromInt64(buyCapacity),
		}
	}

	Convey("Given a funded wallet planning max_fraction of cash", t, func() {
		opportunity := NewOpportunity(
			context.Background(), nil, price, funded, nil, nil,
		)

		Convey("Then a deep touch is charged only the consumed share", func() {
			// feasible = 0.20 * 5000 = 1000 against a 4000 touch: 25%.
			row := forecast(4000)
			opportunity.friction(row)

			So(row.FrictionReady, ShouldBeTrue)
			So(row.ExpectedFees, ShouldAlmostEqual, 0.0026, 1e-12)
			So(row.ExpectedImpact, ShouldAlmostEqual, 0.004*0.25, 1e-12)
		})

		Convey("Then a thin touch is charged the full observed spread", func() {
			row := forecast(500)
			opportunity.friction(row)

			So(row.ExpectedImpact, ShouldAlmostEqual, 0.004, 1e-12)
		})
	})

	Convey("Given a wallet whose cash is unknown", t, func() {
		opportunity := NewOpportunity(
			context.Background(), nil, price, broker.NewBalance(nil, nil, nil),
			nil, nil,
		)

		Convey("Then impact prices the full-touch upper bound", func() {
			row := forecast(4000)
			opportunity.friction(row)

			So(row.ExpectedImpact, ShouldAlmostEqual, 0.004, 1e-12)
		})
	})
}
