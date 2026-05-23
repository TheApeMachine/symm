package pumpdump

import (
	"testing"
	"time"
)

func TestPrecursorScoreRequiresBookAndExecutions(t *testing.T) {
	if precursorScore(0.8, 0.6) <= 0 {
		t.Fatal("expected positive score when book and buys align")
	}

	if precursorScore(0.8, -0.2) != 0 {
		t.Fatal("expected zero score without executed buy confirmation")
	}

	if precursorScore(-0.2, 0.8) != 0 {
		t.Fatal("expected zero score without bid-side book pressure")
	}
}

func TestVolumeSpikeUsesOwnRatioFence(t *testing.T) {
	trackStore := NewTrackStore()
	trackStore.ApplyTicker("PUMP/EUR", 1, 1000)

	for index := 0; index < minVolumeHistory; index++ {
		trackStore.bySymbol["PUMP/EUR"].volumes = append(
			trackStore.bySymbol["PUMP/EUR"].volumes,
			10,
		)
	}

	trackStore.bySymbol["PUMP/EUR"].bucketVolume = 30

	ratio, ok := trackStore.VolumeSpike("PUMP/EUR")

	if !ok {
		t.Fatalf("expected volume spike above own fence, got ratio=%v", ratio)
	}

	trackStore.bySymbol["PUMP/EUR"].bucketVolume = 10

	if _, ok := trackStore.VolumeSpike("PUMP/EUR"); ok {
		t.Fatal("expected no spike at baseline volume")
	}
}

func TestPriceFlatUsesOwnMoveHistory(t *testing.T) {
	trackStore := NewTrackStore()
	track := trackStore.ensure("PUMP/EUR")
	track.priceMoves = []float64{0.02, 0.015, 0.018}
	track.bucketOpenPrice = 1.0
	track.lastPrice = 1.05

	if trackStore.PriceFlat("PUMP/EUR") {
		t.Fatal("expected large move to fail against own quiet history")
	}

	track.lastPrice = 1.005

	if !trackStore.PriceFlat("PUMP/EUR") {
		t.Fatal("expected small move to pass against own quiet history")
	}
}

func TestSpreadTightRequiresCompression(t *testing.T) {
	trackStore := NewTrackStore()

	for _, spread := range []float64{20, 22, 21, 23, 20, 22, 21, 20} {
		trackStore.RecordSpread("PUMP/EUR", spread)
	}

	if !trackStore.SpreadTight("PUMP/EUR", 10) {
		t.Fatal("expected spread below history median to pass")
	}

	if trackStore.SpreadTight("PUMP/EUR", 25) {
		t.Fatal("expected wide spread to fail")
	}
}

func TestLiquidityFilterUsesCrossSectionMedian(t *testing.T) {
	trackStore := NewTrackStore()
	trackStore.ApplyTicker("BTC/EUR", 50000, 100)
	trackStore.ApplyTicker("PUMP/EUR", 1, 500000)

	if trackStore.PassesLiquidity("BTC/EUR") {
		t.Fatal("expected high daily quote volume to sit above the live median")
	}

	if !trackStore.PassesLiquidity("PUMP/EUR") {
		t.Fatal("expected low daily quote volume to sit below the live median")
	}
}

func TestBucketRollClosesFiveMinuteWindow(t *testing.T) {
	trackStore := NewTrackStore()
	start := time.Unix(0, 0)

	trackStore.ApplyTicker("PUMP/EUR", 1, 1000)
	trackStore.AddVolume("PUMP/EUR", 42)
	trackStore.RollBuckets(start)
	trackStore.RollBuckets(start.Add(bucketWindow))

	if len(trackStore.bySymbol["PUMP/EUR"].volumes) != 1 {
		t.Fatalf("expected closed bucket in history, got %d", len(trackStore.bySymbol["PUMP/EUR"].volumes))
	}
}

func TestVolumeRatioFenceUsesSpreadWhenHistoryVaries(t *testing.T) {
	fence := volumeRatioFence([]float64{1, 1.2, 1.4, 1.6})

	if fence <= 1.6 {
		t.Fatalf("expected fence above upper quartile when history varies, got %v", fence)
	}
}

func BenchmarkTrackStoreVolumeSpike(b *testing.B) {
	trackStore := NewTrackStore()
	trackStore.bySymbol["PUMP/EUR"] = &SymbolTrack{
		volumes:      []float64{10, 10, 10, 10},
		bucketVolume: 30,
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, ok := trackStore.VolumeSpike("PUMP/EUR"); !ok {
			b.Fatal("expected spike")
		}
	}
}
