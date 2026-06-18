package integration

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type categoryFixture struct {
	origin        string
	categoryIndex int
	wantSource    logic.SourceType
	wantCategory  logic.CategoryType
}

var signalCategoryFixtures = []categoryFixture{
	{origin: "causal", categoryIndex: 1, wantSource: logic.SourceCausal, wantCategory: logic.CategoryEndogenousAlpha},
	{origin: "correlation", categoryIndex: 1, wantSource: logic.SourceCorrelation, wantCategory: logic.CategorySystemicHerd},
	{origin: "cvd", categoryIndex: 2, wantSource: logic.SourceCVD, wantCategory: logic.CategoryAggressiveDrive},
	{origin: "depthflow", categoryIndex: 1, wantSource: logic.SourceDepthFlow, wantCategory: logic.CategoryLoadedImbalance},
	{origin: "exhaust", categoryIndex: 1, wantSource: logic.SourceExhaustion, wantCategory: logic.CategoryMechanicalCollapse},
	{origin: "fluid", categoryIndex: 1, wantSource: logic.SourceFluid, wantCategory: logic.CategoryLaminar},
	{origin: "hawkes", categoryIndex: 2, wantSource: logic.SourceHawkes, wantCategory: logic.CategorySaturation},
	{origin: "leadlag", categoryIndex: 1, wantSource: logic.SourceLeadLag, wantCategory: logic.CategoryInefficientLag},
	{origin: "liquidity", categoryIndex: 2, wantSource: logic.SourceLiquidity, wantCategory: logic.CategoryMedianDepth},
	{origin: "manifold", categoryIndex: 1, wantSource: logic.SourceManifold, wantCategory: logic.CategorySystemicHerd},
	{origin: "pumpdump", categoryIndex: 3, wantSource: logic.SourcePumpDump, wantCategory: logic.CategoryOrganicTrend},
	{origin: "sentiment", categoryIndex: 1, wantSource: logic.SourceSentiment, wantCategory: logic.CategoryRiskOnSurge},
	{origin: "toxicity", categoryIndex: 1, wantSource: logic.SourceToxicity, wantCategory: logic.CategoryToxicBluff},
}

var measurementOrigins = []string{
	"causal",
	"correlation",
	"cvd",
	"depthflow",
	"exhaust",
	"fluid",
	"hawkes",
	"leadlag",
	"liquidity",
	"manifold",
	"pumpdump",
	"sentiment",
	"toxicity",
}

func testPool(t testing.TB) *qpool.Q[any] {
	t.Helper()

	pool := qpool.NewQ[any](context.Background(), 1, runtime.NumCPU(), &qpool.Config{
		SchedulingTimeout: time.Second,
		Regulators: []qpool.Regulator{
			qpool.NewRegulator(qpool.NewCircuitBreaker(10, 10*time.Second, 10)),
			qpool.NewRegulator(qpool.NewRateLimiter(100, time.Second)),
			qpool.NewRegulator(qpool.NewBackPressureRegulator(1000, time.Second, time.Second)),
			qpool.NewRegulator(qpool.NewResourceGovernorRegulator(90, 90, time.Second)),
		},
	})

	if pool == nil {
		t.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func insertClassifierMeasurement(
	tree *dmt.Tree,
	origin, scope string,
	categoryIndex int,
	confidence float64,
) {
	artifact := datura.Acquire(
		origin, datura.Artifact_Type_json,
	).WithRole(
		"measurement",
	).WithScope(
		scope,
	).Poke(
		categoryIndex, "classifier", "category",
	)
	artifact.Poke(
		confidence, "classifier", "confidence",
	).Poke(
		1.0, "classifier", "strength",
	)

	buf := errnie.Does(func() ([]byte, error) {
		return artifact.Pack()
	}).Value()

	tree.Insert(artifact.Prefix(), buf)
}

func insertTickerQuote(tree *dmt.Tree, symbol string, last, bid, ask float64) {
	payload, _ := json.Marshal(map[string]any{
		"channel": "ticker",
		"type":    "update",
		"data": []map[string]any{{
			"symbol": symbol,
			"last":   last,
			"bid":    bid,
			"ask":    ask,
		}},
	})

	artifact := datura.Acquire(
		"test", datura.Artifact_Type_json,
	).WithRole(
		"ticker",
	).WithScope(
		symbol,
	).WithPayload(
		payload,
	)

	buf := errnie.Does(func() ([]byte, error) {
		return artifact.Pack()
	}).Value()

	tree.Insert(artifact.Prefix(), buf)
}

func measurementTreePrefix(scope, origin string) []byte {
	return []byte("measurement/" + scope + "/" + origin)
}

func ingestMeasurementsFromTree(
	tree *dmt.Tree,
	story *market.Story,
	scopes []string,
) {
	if tree == nil || story == nil {
		return
	}

	for _, scope := range scopes {
		if scope == "" {
			continue
		}

		for _, origin := range measurementOrigins {
			for inbound := range tree.Seek(measurementTreePrefix(scope, origin)) {
				measurement, ok := logic.MeasurementFromArtifact(origin, inbound)

				if !ok {
					continue
				}

				if measurement.Symbol == "" {
					measurement.Symbol, _ = inbound.Scope()
				}

				if measurement.Symbol == "" || measurement.Source == "" {
					continue
				}

				_ = story.Update(inbound)
			}
		}
	}
}

func sortActionsExitsFirst(actions []*logic.Action) []*logic.Action {
	if len(actions) <= 1 {
		return actions
	}

	exits := make([]*logic.Action, 0, len(actions))
	entries := make([]*logic.Action, 0, len(actions))

	for _, action := range actions {
		if action == nil {
			continue
		}

		if action.Type.IsExit() {
			exits = append(exits, action)

			continue
		}

		entries = append(entries, action)
	}

	return append(exits, entries...)
}

func measurementBySource(
	measurements []logic.Measurement,
	source logic.SourceType,
) (logic.Measurement, bool) {
	for _, measurement := range measurements {
		if measurement.Source == source {
			return measurement, true
		}
	}

	return logic.Measurement{}, false
}

func walkTraceHasActionOutcome(trace logic.WalkTrace) bool {
	for _, step := range trace.Steps {
		if step.Outcome == logic.WalkOutcomeAction {
			return true
		}
	}

	return false
}
