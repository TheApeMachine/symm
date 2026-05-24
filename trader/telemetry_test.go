package trader

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

type fakeTelemetryStream struct {
	pulseEvents  []map[string]any
	traceEvents  []map[string]any
	statusEvents []map[string]any
}

func (stream *fakeTelemetryStream) EnginePulse(payload map[string]any) {
	stream.pulseEvents = append(stream.pulseEvents, payload)
}

func (stream *fakeTelemetryStream) DecisionTrace(payload map[string]any) {
	stream.traceEvents = append(stream.traceEvents, payload)
}

func (stream *fakeTelemetryStream) Scoreboard(
	_, _, _ float64,
	_ []map[string]any,
) {
}

func (stream *fakeTelemetryStream) Status(payload map[string]any) {
	stream.statusEvents = append(stream.statusEvents, payload)
}

func runDecisionTick(
	t *testing.T,
	crypto *Crypto,
	measurements []engine.Measurement,
) {
	t.Helper()

	crypto.beginRescoreTick()

	for _, measurement := range measurements {
		crypto.noteCandidate(measurement)
	}

	crypto.mergeLiveCandidates()
	crypto.runExecution(time.Now())
}

func TestTelemetryPublishBuildsEvaluationsAndLine(t *testing.T) {
	originalMinWarm := config.System.MinWarmPulses
	config.System.MinWarmPulses = 0
	t.Cleanup(func() {
		config.System.MinWarmPulses = originalMinWarm
	})

	stream := &fakeTelemetryStream{}
	telemetry := &Telemetry{
		stream:       stream,
		symbolsTotal: 128,
		readings:     make(map[string]symbolReadings),
	}
	crypto, err := NewCrypto(
		context.Background(),
		nil,
		NewWallet(PaperWallet, "EUR", 200, 0.26),
		stubPrices{},
		&stubSignal{},
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	crypto.BindTelemetry(stream, nil, nil, 128)

	telemetry.BeginTick()
	runDecisionTick(t, crypto, []engine.Measurement{
		{
			Source:     "hawkes",
			Type:       engine.Momentum,
			Regime:     "momentum",
			Reason:     "cluster_buy",
			Confidence: 0.8,
			Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
		},
		{
			Source:     "fluid",
			Type:       engine.Flow,
			Regime:     "flow",
			Reason:     "accumulation",
			Confidence: 0.4,
			Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
		},
	})
	telemetry.NoteMeasurement(engine.Measurement{
		Source:     "hawkes",
		Type:       engine.Momentum,
		Regime:     "momentum",
		Reason:     "cluster_buy",
		Confidence: 0.8,
		Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
	})
	telemetry.NoteMeasurement(engine.Measurement{
		Source:     "fluid",
		Type:       engine.Flow,
		Regime:     "flow",
		Reason:     "accumulation",
		Confidence: 0.4,
		Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
	})

	telemetry.Publish(NewWallet(PaperWallet, "EUR", 200, 0.26), crypto)

	if len(stream.statusEvents) == 0 {
		t.Fatal("expected status event")
	}

	if stream.statusEvents[0]["equity_eur"] != 200.0 {
		t.Fatalf("expected wallet balance in status, got %v", stream.statusEvents[0]["equity_eur"])
	}

	if len(stream.pulseEvents) == 0 {
		t.Fatal("expected engine pulse")
	}

	if stream.pulseEvents[0]["measurements"] != 2 {
		t.Fatalf("expected two pulse signals, got %v", stream.pulseEvents[0]["measurements"])
	}

	if avgPrediction, _ := stream.pulseEvents[0]["avg_prediction"].(float64); avgPrediction < 1.19 || avgPrediction > 1.21 {
		t.Fatalf("expected evaluation avg_prediction, got %v", stream.pulseEvents[0]["avg_prediction"])
	}

	if stream.pulseEvents[0]["forecast_symbols"] != 1 {
		t.Fatalf("expected one forecast symbol, got %v", stream.pulseEvents[0]["forecast_symbols"])
	}

	if len(stream.traceEvents) == 0 {
		t.Fatal("expected decision trace")
	}

	evaluations, ok := stream.traceEvents[0]["evaluations"].([]map[string]any)
	if !ok || len(evaluations) != 1 {
		t.Fatalf("expected one evaluation row, got %v", stream.traceEvents[0]["evaluations"])
	}

	if combined, _ := evaluations[0]["combined"].(float64); combined < 1.19 || combined > 1.21 {
		t.Fatalf("expected combined score 1.2, got %v", combined)
	}
}

func TestWhyCodeUsesWarmupAndLine(t *testing.T) {
	if whyCode(true, 1, 0.5) != "field_warming" {
		t.Fatal("expected warming code")
	}

	if whyCode(false, 0.4, 0.9) != "below_line" {
		t.Fatal("expected below_line")
	}

	if whyCode(false, 1.1, 0.9) != "ok" {
		t.Fatal("expected ok")
	}
}

func TestDecisionEngineAllowsCombinedCandidates(t *testing.T) {
	originalMinWarm := config.System.MinWarmPulses
	config.System.MinWarmPulses = 0
	config.System.MinEdgeReturn = 0
	t.Cleanup(func() {
		config.System.MinWarmPulses = originalMinWarm
		config.System.MinEdgeReturn = 0.0005
	})

	crypto, err := NewCrypto(
		context.Background(),
		nil,
		NewWallet(PaperWallet, "EUR", 200, 0.26),
		stubPrices{},
		&scoredSignal{liveScore: 0.42},
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	runDecisionTick(t, crypto, nil)
	crypto.mergeLiveCandidates()
	crypto.runExecution(time.Now())

	snapshot := crypto.DecisionSnapshot()

	if len(snapshot.Evaluations) != 1 {
		t.Fatalf("expected one live evaluation, got %d", len(snapshot.Evaluations))
	}

	if snapshot.Evaluations[0].CombinedScore != 0.42 {
		t.Fatalf("expected combined 0.42, got %v", snapshot.Evaluations[0].CombinedScore)
	}
}

func TestTelemetryPublishIncludesSourceScores(t *testing.T) {
	originalMinWarm := config.System.MinWarmPulses
	config.System.MinWarmPulses = 0
	config.System.MinEdgeReturn = 0
	t.Cleanup(func() {
		config.System.MinWarmPulses = originalMinWarm
		config.System.MinEdgeReturn = 0.0005
	})

	stream := &fakeTelemetryStream{}
	telemetry := &Telemetry{
		stream:       stream,
		symbolsTotal: 128,
		readings:     make(map[string]symbolReadings),
	}
	crypto, err := NewCrypto(
		context.Background(),
		nil,
		NewWallet(PaperWallet, "EUR", 200, 0.26),
		stubPrices{},
		&scoredSignal{liveScore: 0.42},
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	crypto.BindTelemetry(stream, nil, nil, 128)

	telemetry.BeginTick()
	runDecisionTick(t, crypto, nil)
	telemetry.Publish(NewWallet(PaperWallet, "EUR", 200, 0.26), crypto)

	sourceScores, ok := stream.pulseEvents[0]["source_scores"].(map[string]float64)

	if !ok {
		t.Fatalf("expected source_scores map, got %T", stream.pulseEvents[0]["source_scores"])
	}

	if sourceScores["stub"] != 0.42 {
		t.Fatalf("expected stub live score 0.42, got %v", sourceScores["stub"])
	}

	signals, ok := stream.pulseEvents[0]["signals"].([]map[string]any)

	if !ok || len(signals) != 1 {
		t.Fatalf("expected one live pulse signal row, got %v", stream.pulseEvents[0]["signals"])
	}

	evaluations, ok := stream.traceEvents[0]["evaluations"].([]map[string]any)

	if !ok || len(evaluations) != 1 {
		t.Fatalf("expected one evaluation row, got %v", stream.traceEvents[0]["evaluations"])
	}

	if _, ok := stream.statusEvents[0]["positions"]; ok {
		t.Fatal("expected empty positions to be omitted from status")
	}
}

func TestRunExecutionWithoutTelemetry(t *testing.T) {
	originalMinWarm := config.System.MinWarmPulses
	originalMinEdge := config.System.MinEdgeReturn
	config.System.MinWarmPulses = 0
	config.System.MinEdgeReturn = 0
	t.Cleanup(func() {
		config.System.MinWarmPulses = originalMinWarm
		config.System.MinEdgeReturn = originalMinEdge
	})

	crypto, err := NewCrypto(
		context.Background(),
		nil,
		NewWallet(PaperWallet, "EUR", 200, 0.26),
		stubPrices{"PUMP/EUR": 1.0},
		&stubSignal{},
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	runDecisionTick(t, crypto, []engine.Measurement{
		{
			Source:         "hawkes",
			Type:           engine.Momentum,
			Regime:         "momentum",
			Reason:         "cluster_buy",
			Confidence:     0.9,
			ExpectedReturn: 0.01,
			Pairs:          []asset.Pair{{Wsname: "PUMP/EUR"}},
		},
	})

	snapshot := crypto.DecisionSnapshot()

	if len(snapshot.Evaluations) == 0 {
		t.Fatal("expected evaluation rows without telemetry")
	}

	if !snapshot.Evaluations[0].Allow {
		t.Fatalf("expected allow=true, got why=%q", snapshot.Evaluations[0].Why)
	}

	if crypto.portfolio.Status(stubPrices{"PUMP/EUR": 1.0}).OpenCount != 1 {
		t.Fatal("expected portfolio to open one position without telemetry")
	}
}
