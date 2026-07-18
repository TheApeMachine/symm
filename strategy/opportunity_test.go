package strategy

import (
	"testing"
	"time"

	"github.com/theapemachine/nomagique/physics/manifold"
	symmmanifold "github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

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
