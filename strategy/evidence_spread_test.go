package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
TestProjectManifoldSpreadStaysReturnSpace ensures GasReady spread is not
re-divided by ReferencePrice (manifold already stores (ask-bid)/mid).
*/
func TestProjectManifoldSpreadStaysReturnSpace(t *testing.T) {
	t.Parallel()

	thesis := types.NewThesis(nil, nil)
	thesis.Manifold.Store("AAA/USD", manifold.State{
		Source:         "manifold",
		Symbol:         "AAA/USD",
		At:             time.Unix(1, 0).UTC(),
		Duration:       time.Second,
		Epoch:          1,
		ReferencePrice: 100,
		Spread:         0.004,
		BuyCapacity:    50,
		SellCapacity:   50,
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
	})

	evidence := NewEvidence().Project(thesis, types.Holding{
		Symbol:     "AAA/USD",
		Mark:       decimal.NewFromFloat64(100),
		EntryPrice: decimal.NewFromFloat64(100),
	})

	if evidence.Spread != 0.004 {
		t.Fatalf("want return-space spread 0.004, got %v", evidence.Spread)
	}
}
