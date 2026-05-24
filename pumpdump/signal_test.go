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

func TestMeanConfidence(t *testing.T) {
	pumpdumpSignal := &PumpDump{track: &TrackStore{}}

	pumpdumpSignal.track.ObserveGaugeScore(0.2)
	pumpdumpSignal.track.ObserveGaugeScore(0.6)

	if got := pumpdumpSignal.MeanConfidence(); got < 0.399 || got > 0.401 {
		t.Fatalf("expected mean confidence 0.4, got %v", got)
	}
}

func TestVolumeSpikeUsesOwnRatioFence(t *testing.T) {
	trackStore, pool := testTrackStore(t)
	filter := &PrecursorFilter{}
	publishTick(pool, "PUMP/EUR", 1, 1000)
	drainTrack(trackStore)

	for index := 0; index < minVolumeHistory; index++ {
		trackStore.ensure("PUMP/EUR").volumes = append(
			trackStore.ensure("PUMP/EUR").volumes,
			10,
		)
	}

	trackStore.ensure("PUMP/EUR").bucketVolume = 30

	ratio, ok := filter.volumeSpike("PUMP/EUR", trackStore)

	if !ok {
		t.Fatalf("expected volume spike above own fence, got ratio=%v", ratio)
	}

	trackStore.ensure("PUMP/EUR").bucketVolume = 10

	if _, ok := filter.volumeSpike("PUMP/EUR", trackStore); ok {
		t.Fatal("expected no spike at baseline volume")
	}
}

func TestPriceFlatUsesOwnMoveHistory(t *testing.T) {
	trackStore, _ := testTrackStore(t)
	filter := &PrecursorFilter{}
	track := trackStore.ensure("PUMP/EUR")
	track.priceMoves = []float64{0.02, 0.015, 0.018}
	track.bucketOpenPrice = 1.0
	track.lastPrice = 1.05

	if filter.priceFlat("PUMP/EUR", trackStore) {
		t.Fatal("expected large move to fail against own quiet history")
	}

	track.lastPrice = 1.005

	if !filter.priceFlat("PUMP/EUR", trackStore) {
		t.Fatal("expected small move to pass against own quiet history")
	}
}

func TestSpreadTightRequiresCompression(t *testing.T) {
	trackStore, pool := testTrackStore(t)
	filter := &PrecursorFilter{}

	for _, spread := range []float64{20, 22, 21, 23, 20, 22, 21, 20} {
		publishBook(pool, "PUMP/EUR", spread)
	}

	drainTrack(trackStore)

	if !filter.spreadTight("PUMP/EUR", trackStore, 10) {
		t.Fatal("expected spread below history median to pass")
	}

	if filter.spreadTight("PUMP/EUR", trackStore, 25) {
		t.Fatal("expected wide spread to fail")
	}
}

func TestLiquidityFilterUsesCrossSectionMedian(t *testing.T) {
	trackStore, pool := testTrackStore(t)
	filter := &PrecursorFilter{}
	publishTick(pool, "BTC/EUR", 50000, 100)
	publishTick(pool, "PUMP/EUR", 1, 500000)
	drainTrack(trackStore)

	if filter.passesLiquidity("BTC/EUR", trackStore) {
		t.Fatal("expected high daily quote volume to sit above the live median")
	}

	if !filter.passesLiquidity("PUMP/EUR", trackStore) {
		t.Fatal("expected low daily quote volume to sit below the live median")
	}
}

func TestBucketRollClosesFiveMinuteWindow(t *testing.T) {
	trackStore, pool := testTrackStore(t)
	start := time.Unix(0, 0)

	publishTick(pool, "PUMP/EUR", 1, 1000)
	publishTrade(pool, "PUMP/EUR", 42, start)
	drainTrack(trackStore)

	trackStore.RollBuckets(start)
	trackStore.RollBuckets(start.Add(bucketWindow))

	if len(trackStore.ensure("PUMP/EUR").volumes) != 1 {
		t.Fatalf("expected closed bucket in history, got %d", len(trackStore.ensure("PUMP/EUR").volumes))
	}
}

func TestVolumeRatioFenceUsesSpreadWhenHistoryVaries(t *testing.T) {
	fence := volumeRatioFence([]float64{1, 1.2, 1.4, 1.6})

	if fence <= 1.6 {
		t.Fatalf("expected fence above upper quartile when history varies, got %v", fence)
	}
}

func BenchmarkPrecursorFilterVolumeSpike(b *testing.B) {
	trackStore, _ := testTrackStore(&testing.T{})
	filter := &PrecursorFilter{}
	trackStore.ensure("PUMP/EUR").volumes = []float64{10, 10, 10, 10}
	trackStore.ensure("PUMP/EUR").bucketVolume = 30

	b.ReportAllocs()

	for b.Loop() {
		if _, ok := filter.volumeSpike("PUMP/EUR", trackStore); !ok {
			b.Fatal("expected spike")
		}
	}
}
