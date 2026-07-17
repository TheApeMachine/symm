package strategy

import (
	"testing"
	"time"

	"github.com/theapemachine/nomagique/physics/manifold"
	symmmanifold "github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
TestOpportunityReservedNeedsMarginAndLead requires both positive economic edge
and positive cognitive lead; SNR alone or lead alone is not reserved.
*/
func TestOpportunityReservedNeedsMarginAndLead(t *testing.T) {
	t.Parallel()

	thesis := types.NewThesis(nil, nil)
	thesis.Manifold.Store("OXT/USD", readyBasin("OXT/USD", 0.2))

	forecast := decideForecast("OXT/USD", 0.08, 0.02)
	cognition := buyCognition("OXT/USD")
	cognition.Confidence = 0.7

	reading := measureOpportunity(forecast, cognition, thesis)

	if reading.Margin <= 0 || reading.Lead <= 0 || !reading.Reserved() {
		t.Fatalf("want reserved-ready reading, got %+v", reading)
	}

	snrOnly := measureOpportunity(
		decideForecast("OXT/USD", 0.08, 0.02),
		buyCognition("OXT/USD"), // confidence 0.6, basin 0.7 → negative lead
		func() *types.Thesis {
			cut := types.NewThesis(nil, nil)
			cut.Manifold.Store("OXT/USD", readyBasin("OXT/USD", 0.7))
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

func readyBasin(symbol string, coherence float64) symmmanifold.State {
	return symmmanifold.State{
		Source:         "manifold",
		Symbol:         symbol,
		At:             time.Unix(1, 0).UTC(),
		Duration:       time.Second,
		Epoch:          1,
		ReferencePrice: 1,
		Spread:         0.001,
		BuyCapacity:    1000,
		SellCapacity:   1000,
		InvalidReason:  symmmanifold.Valid,
		BuyIntensity:   1,
		SellIntensity:  0.5,
		SpectralRadius: 0.1,
		Reading: manifold.Reading{
			PressureGradX: 0.1,
			Divergence:    -0.1,
			CoherenceMag2: coherence,
			GuidanceSpeed: 0.1,
		},
	}
}
