package pumpdump

import (
	"testing"

	"github.com/theapemachine/symm/kraken/asset"
)

func TestWarmHourlyVolumeBaselineEmptyPair(t *testing.T) {
	_, err := WarmHourlyVolumeBaseline(asset.Pair{})

	if err == nil {
		t.Fatal("expected error for empty pair")
	}
}
