package fluid

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

type fluidMarketStub struct {
	latest map[string]engine.Snapshot
	fresh  map[string]engine.Snapshot
}

func (stub *fluidMarketStub) Read(symbol string) engine.Snapshot {
	return stub.latest[symbol]
}

func (stub *fluidMarketStub) ReadFresh(
	symbol string,
	_ time.Time,
	_ time.Duration,
) engine.Snapshot {
	return stub.fresh[symbol]
}

func TestContinuitySourceDetectsHiddenAccumulation(t *testing.T) {
	prior := fieldSample{density: 10, flow: 0}
	current := fieldSample{density: 14, flow: 8}

	source := continuitySource(current, prior)

	if source <= 0 {
		t.Fatalf("expected positive source term, got %v", source)
	}
}

func TestBurgersShockSpikesOnZeroViscosity(t *testing.T) {
	prior := fieldSample{velocity: 0.001, viscosity: 10}
	current := fieldSample{velocity: 0.02, viscosity: 0}

	shock := burgersShock(current, prior)

	if shock <= burgersShock(fieldSample{velocity: 0.02, viscosity: minViscosityEpsilon}, prior) {
		t.Fatalf("expected zero-viscosity shock to spike above epsilon floor, got %v", shock)
	}
}

func TestBurgersShockRequiresVelocityJump(t *testing.T) {
	prior := fieldSample{velocity: 0.001, viscosity: 10}
	current := fieldSample{velocity: 0.02, viscosity: 10}

	shock := burgersShock(current, prior)

	if shock <= 0 {
		t.Fatal("expected positive shock term")
	}

	steady := fieldSample{velocity: 0.02, viscosity: 10}

	if burgersShock(steady, current) != 0 {
		t.Fatal("expected zero shock without velocity jump")
	}
}

func TestTrackStoreFiresOnAccumulationWithQuietVelocity(t *testing.T) {
	trackStore := NewTrackStore()
	trackStore.ApplyTicker("PUMP/EUR", 1, 500000)
	trackStore.ApplyTicker("BTC/EUR", 50000, 100)

	start := time.Unix(0, 0)

	for index := 0; index < minFieldHistory+1; index++ {
		at := start.Add(time.Duration(index) * time.Second)
		_, _ = trackStore.Sample("PUMP/EUR", 10, 1, 20, 0, 1, 0.2, at)
	}

	track := trackStore.bySymbol["PUMP/EUR"]

	for index := 0; index < minFieldHistory; index++ {
		track.sourceHistory = append(track.sourceHistory, 0.5)
		track.shockHistory = append(track.shockHistory, 0.001)
	}

	track.confidenceHistory = []float64{0.2, 0.4, 0.6, 0.8, 1.0, 1.2, 1.4, 1.6, 1.8, 2.0, 2.2, 2.4}

	at := start.Add(time.Duration(minFieldHistory+1) * time.Second)
	confidence, reason := trackStore.Sample("PUMP/EUR", 25, 1, 5, 0, 20, 0.9, at)

	if confidence <= 0 {
		t.Fatalf("expected fluid confidence, got %v", confidence)
	}

	if reason != "accumulation" && reason != "shock" {
		t.Fatalf("expected accumulation or shock reason, got %q", reason)
	}
}

func TestFieldConfidenceRequiresBuyPressure(t *testing.T) {
	if fieldConfidence(2, 0, -0.2, true) != 0 {
		t.Fatal("expected zero confidence without buy-side pressure")
	}

	if fieldConfidence(2, 0, 0.8, true) <= 0 {
		t.Fatal("expected accumulation confidence with quiet source term")
	}
}

func TestMeanConfidence(t *testing.T) {
	fluidSignal := &Fluid{track: NewTrackStore()}

	fluidSignal.track.ObserveGaugeScore(0.2)
	fluidSignal.track.ObserveGaugeScore(0.6)

	if got := fluidSignal.MeanConfidence(); got < 0.399 || got > 0.401 {
		t.Fatalf("expected mean confidence 0.4, got %v", got)
	}
}

func TestMeasureSamplesLatestFieldForDashboard(t *testing.T) {
	convey.Convey("Given latest fluid state but no executable fresh trade snapshot", t, func() {
		start := time.Unix(1_700_000_000, 0)
		marketStub := &fluidMarketStub{
			latest: map[string]engine.Snapshot{},
			fresh:  map[string]engine.Snapshot{},
		}
		fluidSignal := &Fluid{
			market: marketStub,
			track:  NewTrackStore(),
			pairs: map[string]asset.Pair{
				"ALT/EUR": {Wsname: "ALT/EUR"},
				"BTC/EUR": {Wsname: "BTC/EUR"},
			},
			symbols: []string{"ALT/EUR", "BTC/EUR"},
		}
		fluidSignal.track.ApplyTicker("ALT/EUR", 10, 100)
		fluidSignal.track.ApplyTicker("BTC/EUR", 100, 1000)

		marketStub.latest["ALT/EUR"] = fluidSnapshot(10, 100, start)
		marketStub.latest["BTC/EUR"] = fluidSnapshot(100, 1000, start)
		marketStub.fresh["ALT/EUR"] = staleFluidSnapshot(10, 100, start)
		marketStub.fresh["BTC/EUR"] = staleFluidSnapshot(100, 1000, start)

		measurements := collectFluidMeasurements(fluidSignal, start)

		marketStub.latest["ALT/EUR"] = fluidSnapshot(10.2, 100, start.Add(time.Second))
		marketStub.latest["BTC/EUR"] = fluidSnapshot(100.1, 1000, start.Add(time.Second))
		marketStub.fresh["ALT/EUR"] = staleFluidSnapshot(10.2, 100, start.Add(time.Second))
		marketStub.fresh["BTC/EUR"] = staleFluidSnapshot(100.1, 1000, start.Add(time.Second))

		measurements = append(measurements, collectFluidMeasurements(
			fluidSignal, start.Add(time.Second),
		)...)

		convey.Convey("It should update field samples without emitting stale candidates", func() {
			convey.So(measurements, convey.ShouldHaveLength, 0)
			convey.So(fluidSignal.SampledCount(), convey.ShouldEqual, 1)
		})
	})
}

func TestSampleIgnoresNonIncreasingTime(t *testing.T) {
	convey.Convey("Given an existing field sample timestamp", t, func() {
		trackStore := NewTrackStore()
		trackStore.ApplyTicker("ALT/EUR", 10, 100)
		trackStore.ApplyTicker("BTC/EUR", 100, 1000)
		start := time.Unix(1_700_000_000, 0)

		_, _ = trackStore.Sample("ALT/EUR", 10, 10, 1, 0, 1, 0.5, start)
		_, _ = trackStore.Sample("ALT/EUR", 11, 10.1, 1, 0, 1, 0.5, start.Add(time.Second))
		_, _ = trackStore.Sample("ALT/EUR", 12, 10.2, 1, 0, 1, 0.5, start.Add(time.Second))

		convey.Convey("It should not append duplicate samples from the same market event", func() {
			convey.So(trackStore.SampledCount(), convey.ShouldEqual, 1)

			track := trackStore.bySymbol["ALT/EUR"]
			convey.So(track.samples, convey.ShouldHaveLength, 1)
		})
	})
}

func collectFluidMeasurements(fluidSignal *Fluid, now time.Time) []engine.Measurement {
	measurements := make([]engine.Measurement, 0)

	for measurement := range fluidSignal.Measure(context.Background(), now) {
		measurements = append(measurements, measurement)
	}

	return measurements
}

func fluidSnapshot(last, volumeBase float64, at time.Time) engine.Snapshot {
	return engine.Snapshot{
		Last:        last,
		LastAt:      at,
		LastOK:      true,
		VolumeBase:  volumeBase,
		VolumeOK:    true,
		BatchVolume: 2,
		TradesAt:    at,
		BatchOK:     true,
		BuyPressure: 0.6,
		PressureOK:  true,
		SpreadBPS:   1,
		BookAt:      at,
		SpreadOK:    true,
		Density:     10,
		DensityOK:   true,
	}
}

func staleFluidSnapshot(last, volumeBase float64, at time.Time) engine.Snapshot {
	snapshot := fluidSnapshot(last, volumeBase, at)
	snapshot.BatchOK = false
	snapshot.PressureOK = false

	return snapshot
}

func BenchmarkContinuitySource(b *testing.B) {
	prior := fieldSample{density: 10, flow: 1}
	current := fieldSample{density: 15, flow: 10}

	b.ReportAllocs()

	for b.Loop() {
		if continuitySource(current, prior) <= 0 {
			b.Fatal("expected source")
		}
	}
}
