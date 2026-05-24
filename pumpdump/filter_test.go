package pumpdump

import (
	"testing"

	"github.com/theapemachine/symm/engine"
)

func TestPrecursorFilterRejectsUnconfirmedScore(t *testing.T) {
	trackStore, _ := testTrackStore(t)
	track := trackStore.ensure("PUMP/EUR")
	track.dailyQuoteVol = 1000
	trackStore.ensure("BTC/EUR").dailyQuoteVol = 50000
	track.volumes = []float64{1, 1, 1, 1}
	track.bucketVolume = 2

	filter := &PrecursorFilter{}
	snapshot := engine.Snapshot{
		ImbalanceOK: true,
		Imbalance:   0.8,
		PressureOK:  true,
		BuyPressure: 0.6,
	}

	confidence, reason := filter.Score("PUMP/EUR", trackStore, snapshot, track.bucketStart)

	if confidence != 0 {
		t.Fatalf("expected zero precursor confidence, got %v", confidence)
	}

	if reason != "precursor" {
		t.Fatalf("expected precursor reason, got %q", reason)
	}
}

func TestPrecursorFilterReturnsScoreForActualPump(t *testing.T) {
	trackStore, _ := testTrackStore(t)
	track := trackStore.ensure("PUMP/EUR")
	track.dailyQuoteVol = 1000
	trackStore.ensure("BTC/EUR").dailyQuoteVol = 50000
	track.volumes = []float64{1, 1, 1, 1, 1, 1, 1, 1}
	track.bucketVolume = 20
	track.bucketOpenPrice = 1
	track.lastPrice = 1.001
	track.priceMoves = []float64{0.02, 0.015, 0.018}
	track.spreads = []float64{30, 28, 26, 24, 22, 20, 18, 16}

	filter := &PrecursorFilter{}
	snapshot := engine.Snapshot{
		ImbalanceOK: true,
		Imbalance:   0.8,
		PressureOK:  true,
		BuyPressure: 0.6,
		SpreadOK:    true,
		SpreadBPS:   10,
	}

	confidence, reason := filter.Score("PUMP/EUR", trackStore, snapshot, track.bucketStart)

	if confidence <= 0 {
		t.Fatalf("expected actual pump confidence, got %v", confidence)
	}

	if reason != "actual_pump" {
		t.Fatalf("expected actual_pump reason, got %q", reason)
	}
}
