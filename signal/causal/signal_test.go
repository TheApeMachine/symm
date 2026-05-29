package causal

import (
	"math"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestBackdoorBlocksMacroConfounder(t *testing.T) {
	samples := make([]causalSample, 0, 32)

	for index := 0; index < 32; index++ {
		macro := float64(index%4) * 0.01
		liquidity := 1 + float64(index%3)*0.1
		flow := macro*50 + float64(index%5)*0.2
		velocity := macro*40 + flow*0.2

		samples = append(samples, newCausalSample(macro, liquidity, flow, velocity))
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
		samples = append(samples, newCausalSample(
			float64(index%4)*0.005,
			1+float64(index%3)*0.1,
			float64(index)*0.5,
			float64(index)*0.1,
		))
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

func TestCausalMeasureTickerOnly(t *testing.T) {
	state := NewCausalSymbol(asset.Pair{Wsname: "ALT/EUR"}, engine.DefaultCalibrationParams())
	state.lastPrice = 10
	state.bid = 9.99
	state.ask = 10.01
	state.changePct = 2.5

	measurement, ok := state.Measure(1.2, 0, time.Now())

	if !ok {
		t.Fatal("expected ticker-only causal measurement")
	}

	if measurement.Confidence <= 0 || measurement.Reason != "macro_association" {
		t.Fatalf("unexpected ticker-only measurement: %+v", measurement)
	}
}

func TestSymbolEvaluateIntervention(t *testing.T) {
	state := NewCausalSymbol(asset.Pair{Wsname: "ALT/EUR"}, engine.DefaultCalibrationParams())

	for index := 0; index < minCausalHistory; index++ {
		state.samples = append(state.samples, newCausalSample(
			float64(index%4)*0.005,
			1+float64(index%3)*0.15,
			float64(index)*0.4,
			float64(index)*0.08,
		))
	}

	for _, effect := range []float64{0.1, 0.2, 0.3, 0.4} {
		state.recordIntervention(regimeNormal, effect)
	}

	sample := newCausalSample(0.015, 2.0, 2.0, 0.2)

	confidence, reason := state.evaluate(sample, 0)

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

func TestAssociationEffectReturnsPearson(t *testing.T) {
	samples := []causalSample{
		newCausalSample(0, 0, 1, 2),
		newCausalSample(0, 0, 2, 4),
		newCausalSample(0, 0, 3, 6),
	}

	correlation := associationEffect(samples)

	if correlation <= 0.99 {
		t.Fatalf("expected strong positive correlation, got %v", correlation)
	}
}

func TestOpportunityRunway(t *testing.T) {
	samples := []causalSample{
		newCausalSample(0, 0, 0, 0.01),
		newCausalSample(0, 0, 0, 0.02),
		newCausalSample(0, 0, 0, 0.08),
	}

	convey.Convey("Given excess velocity versus history", t, func() {
		convey.Convey("It should shorten the runway", func() {
			runway := opportunityRunway(samples, time.Second)

			convey.So(runway, convey.ShouldBeLessThan, time.Second)
			convey.So(runway, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkBackdoorFlowEffect(b *testing.B) {
	samples := make([]causalSample, 0, causalHistoryCap)

	for index := 0; index < causalHistoryCap; index++ {
		samples = append(samples, newCausalSample(
			float64(index%5)*0.01,
			1+float64(index%4)*0.2,
			float64(index)*0.3,
			float64(index)*0.05,
		))
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
		samples = append(samples, newCausalSample(
			float64(index%5)*0.01,
			1+float64(index%4)*0.2,
			float64(index)*0.3,
			float64(index)*0.05,
		))
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, ok := fitStructural(samples); !ok {
			b.Fatal("expected fit")
		}
	}
}
