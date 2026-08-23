package hindsight

import (
	"testing"
	"time"
)

func TestCollectDecisionsIndexLatestReplayWins(t *testing.T) {
	at := time.Unix(42, 0).UTC()
	index := collectDecisionsIndex([]Decision{
		{Symbol: "DENT/USD", At: at, Action: "enter", ThesisScore: 0.2},
		{Symbol: "DENT/USD", At: at, Action: "nothing", ThesisScore: 0.9},
	})

	decisions := index["DENT/USD"]
	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decisions))
	}
	if decisions[0].Action != "nothing" || decisions[0].ThesisScore != 0.9 {
		t.Fatalf("latest decision not retained: %#v", decisions[0])
	}
}
