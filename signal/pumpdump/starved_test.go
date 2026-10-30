package pumpdump

import (
	"testing"

	"github.com/theapemachine/symm/signal/volumeclock"
)

func TestPublishedStarvedRegimeAndColdClock(t *testing.T) {
	state := &symbolState{
		lastStarved:   0.8,
		starvedRegime: 0.95,
		clock:         volumeclock.Clock{CompletedBars: 2},
	}

	if got := publishedStarved(state, 0.1); got < 0.95 {
		t.Fatalf("expected max(instant, regime), got %v", got)
	}

	state.clock.CompletedBars = 0
	if publishedStarved(state, 1) != 0 {
		t.Fatal("expected cold volume clock to zero published starved")
	}
}
