package audit

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

func TestConcurrentObserverRetainsPeakExcursionOnce(t *testing.T) {
	stager := NewStager(nil)
	provider := &mockPriceProvider{price: 100}
	feedback := &mockObserverFeedback{}
	observer := NewConcurrentObserver(stager, provider, feedback)
	observer.thresholdPct = 0.02

	decision := &types.Decision{
		ID:     "peak-decision",
		Symbol: "DENT/USD",
		Action: types.ActionNothing,
		At:     time.Now().UTC(),
		Mark:   decimal.NewFromFloat64(100),
	}
	stager.Stage(decision, 20*time.Millisecond)

	provider.price = 160
	observer.samplePending()
	provider.price = 101
	time.Sleep(25 * time.Millisecond)
	observer.samplePending()
	observer.evaluateMatured()

	if len(feedback.hindsight) != 1 {
		t.Fatalf("hindsight feedback count = %d, want 1", len(feedback.hindsight))
	}
	if got := feedback.hindsight[0].MissedReturn; got < 0.599999 || got > 0.600001 {
		t.Fatalf("missed return = %.8f, want 0.6", got)
	}

	observer.evaluateMatured()
	if len(feedback.hindsight) != 1 {
		t.Fatalf("duplicate feedback count = %d, want 1", len(feedback.hindsight))
	}
}
