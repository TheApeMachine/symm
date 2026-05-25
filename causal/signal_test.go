package causal

import (
	"math"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestBackdoorBlocksMacroConfounder(t *testing.T) {
	samples := make([]causalSample, 0, 32)

	for index := 0; index < 32; index++ {
		macro := float64(index%4) * 0.01
		liquidity := 1 + float64(index%3)*0.1
		flow := macro*50 + float64(index%5)*0.2
		velocity := macro*40 + flow*0.2

		samples = append(samples, causalSample{
			macroMomentum: macro,
			liquidity:     liquidity,
			localFlow:     flow,
			priceVelocity: velocity,
		})
	}

	association := associationEffect(samples)
	intervention := backdoorFlowEffect(samples)

	if intervention <= 0 {
		t.Fatalf("expected positive backdoor effect, got %v", intervention)
	}

	if math.Abs(intervention-association) <= 0.01 {
		t.Fatalf("expected confounding gap, association=%v intervention=%v", association, intervention)
	}
}

func TestCounterfactualUpliftPositive(t *testing.T) {
	samples := make([]causalSample, 0, minCausalHistory)

	for index := 0; index < minCausalHistory; index++ {
		samples = append(samples, causalSample{
			macroMomentum: float64(index%4) * 0.005,
			liquidity:     1 + float64(index%3)*0.1,
			localFlow:     float64(index) * 0.5,
			priceVelocity: float64(index) * 0.1,
		})
	}

	coef, ok := fitStructural(samples)

	if !ok {
		t.Fatal("expected structural fit")
	}

	current := samples[6]
	uplift := counterfactualUplift(current, coef, flowInterventionLevel(samples))

	if uplift <= 0 {
		t.Fatalf("expected positive counterfactual uplift, got %v", uplift)
	}
}

func TestTrackStoreFiresOnIntervention(t *testing.T) {
	trackStore := NewTrackStore(engine.DefaultCalibrationParams())
	trackStore.ApplyTicker("ALT/EUR", 1, 500000)
	trackStore.ApplyTicker("BTC/EUR", 50000, 1000000)

	track := trackStore.ensure("ALT/EUR")
	track.hasPrior = true
	track.lastElapsed = time.Second

	for index := 0; index < minCausalHistory; index++ {
		track.samples = append(track.samples, causalSample{
			macroMomentum: float64(index%4) * 0.005,
			liquidity:     1 + float64(index%3)*0.15,
			localFlow:     float64(index) * 0.4,
			priceVelocity: float64(index) * 0.08,
		})
	}

	track.confidenceHistory = []float64{0.2, 0.4, 0.6, 0.8, 1.0, 1.2, 1.4, 1.6, 1.8, 2.0, 2.2, 2.4}

	sample := causalSample{
		macroMomentum: 0.015,
		liquidity:     2.0,
		localFlow:     2.0,
		priceVelocity: 0.2,
	}

	confidence, reason := trackStore.Evaluate("ALT/EUR", sample)

	if confidence <= 0 {
		t.Fatalf("expected causal confidence, got %v", confidence)
	}

	if reason == "" {
		t.Fatalf("expected causal reason, got %q", reason)
	}

	if reason != "intervention" && reason != "counterfactual_like" {
		t.Fatalf("expected intervention reason, got %q", reason)
	}
}

func TestMeanConfidence(t *testing.T) {
	causalSignal := &Causal{track: NewTrackStore(engine.DefaultCalibrationParams())}

	causalSignal.track.ObserveGaugeScore(0.2)
	causalSignal.track.ObserveGaugeScore(0.6)

	if got := causalSignal.MeanConfidence(); got < 0.399 || got > 0.401 {
		t.Fatalf("expected mean confidence 0.4, got %v", got)
	}
}

func TestEvaluateAppliesTickerBeforeLiquidityGate(t *testing.T) {
	convey.Convey("Given an unseen symbol with a complete causal snapshot", t, func() {
		causalSignal := &Causal{track: NewTrackStore(engine.DefaultCalibrationParams())}
		snapshot := engine.Snapshot{
			Last:        10,
			LastOK:      true,
			VolumeBase:  100,
			VolumeOK:    true,
			BatchVolume: 2,
			BatchOK:     true,
			BuyPressure: 0.5,
			PressureOK:  true,
			SpreadBPS:   1,
			SpreadOK:    true,
			Imbalance:   0.4,
			ImbalanceOK: true,
		}

		causalSignal.evaluate("ALT/EUR", snapshot, 0.01, time.Unix(1_700_000_000, 0))

		causalSignal.track.shard.LockMap()
		track := causalSignal.track.bySymbol["ALT/EUR"]
		quoteVolume := 0.0

		if track != nil {
			quoteVolume = track.dailyQuoteVol
		}

		causalSignal.track.shard.UnlockMap()

		convey.Convey("It should record quote volume before applying liquidity gates", func() {
			convey.So(quoteVolume, convey.ShouldAlmostEqual, 1000.0)
		})
	})
}

func TestEvaluateAccumulatesWarmSamples(t *testing.T) {
	convey.Convey("Given repeated complete causal snapshots before history is ready", t, func() {
		causalSignal := &Causal{track: NewTrackStore(engine.DefaultCalibrationParams())}
		causalSignal.track.ApplyTicker("BTC/EUR", 100, 10000)
		start := time.Unix(1_700_000_000, 0)

		for index := range 3 {
			snapshot := engine.Snapshot{
				Last:        10 + float64(index)*0.1,
				LastOK:      true,
				VolumeBase:  100,
				VolumeOK:    true,
				BatchVolume: 2 + float64(index)*0.1,
				BatchOK:     true,
				BuyPressure: 0.5,
				PressureOK:  true,
				SpreadBPS:   1,
				SpreadOK:    true,
				Imbalance:   0.4,
				ImbalanceOK: true,
			}

			causalSignal.evaluate(
				"ALT/EUR",
				snapshot,
				0.01,
				start.Add(time.Duration(index)*time.Second),
			)
		}

		causalSignal.track.shard.LockMap()
		track := causalSignal.track.bySymbol["ALT/EUR"]
		samples := 0

		if track != nil {
			samples = len(track.samples)
		}

		causalSignal.track.shard.UnlockMap()

		convey.Convey("It should keep warming the causal history", func() {
			convey.So(samples, convey.ShouldEqual, 3)
		})
	})
}

func TestEvaluateProducesConfidenceAfterWarmSamples(t *testing.T) {
	convey.Convey("Given flow and velocity co-move through the causal warmup", t, func() {
		causalSignal := &Causal{track: NewTrackStore(engine.DefaultCalibrationParams())}
		causalSignal.track.ApplyTicker("BTC/EUR", 100, 10000)
		start := time.Unix(1_700_000_000, 0)
		confidence := 0.0

		for index := range minCausalHistory + engine.DefaultCalibrationParams().MinConfidenceHistory + 4 {
			step := float64(index)
			snapshot := engine.Snapshot{
				Last:        10 + step*step*0.01,
				LastOK:      true,
				VolumeBase:  100,
				VolumeOK:    true,
				BatchVolume: 1 + step*0.4,
				BatchOK:     true,
				BuyPressure: 0.6,
				PressureOK:  true,
				SpreadBPS:   1,
				SpreadOK:    true,
				Imbalance:   0.4,
				ImbalanceOK: true,
			}

			confidence, _ = causalSignal.evaluate(
				"ALT/EUR",
				snapshot,
				0.01,
				start.Add(time.Duration(index)*time.Second),
			)
		}

		convey.Convey("It should emit a normalized causal confidence", func() {
			convey.So(confidence, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestAssociationEffectReturnsPearson(t *testing.T) {
	samples := []causalSample{
		{localFlow: 1, priceVelocity: 2},
		{localFlow: 2, priceVelocity: 4},
		{localFlow: 3, priceVelocity: 6},
	}

	correlation := associationEffect(samples)

	if correlation <= 0.99 {
		t.Fatalf("expected strong positive correlation, got %v", correlation)
	}
}

func BenchmarkBackdoorFlowEffect(b *testing.B) {
	samples := make([]causalSample, 0, causalHistoryCap)

	for index := 0; index < causalHistoryCap; index++ {
		samples = append(samples, causalSample{
			macroMomentum: float64(index%5) * 0.01,
			liquidity:     1 + float64(index%4)*0.2,
			localFlow:     float64(index) * 0.3,
			priceVelocity: float64(index) * 0.05,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		if backdoorFlowEffect(samples) <= 0 {
			b.Fatal("expected effect")
		}
	}
}

func BenchmarkFitStructural(b *testing.B) {
	samples := make([]causalSample, 0, causalHistoryCap)

	for index := 0; index < causalHistoryCap; index++ {
		samples = append(samples, causalSample{
			macroMomentum: float64(index%5) * 0.01,
			liquidity:     1 + float64(index%4)*0.2,
			localFlow:     float64(index) * 0.3,
			priceVelocity: float64(index) * 0.05,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, ok := fitStructural(samples); !ok {
			b.Fatal("expected fit")
		}
	}
}
