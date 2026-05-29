package sentiment

import "testing"

func TestLeaderPeers(t *testing.T) {
	peers := leaderPeers(map[string]float64{
		"A/EUR": 0.1,
		"B/EUR": 0.2,
	}, "A/EUR")

	if len(peers) != 1 || peers[0] != 0.2 {
		t.Fatalf("expected B/EUR peer only, got %v", peers)
	}
}
