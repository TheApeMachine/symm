package reasoning

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func productionPlaybookPath(t testing.TB) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	return filepath.Join(filepath.Dir(file), "..", "cfg", "perspectives.yaml")
}

func loadProductionPlaybook(t *testing.T) []Thought {
	t.Helper()

	raw, err := os.ReadFile(productionPlaybookPath(t))
	if err != nil {
		t.Fatalf("read production playbook: %v", err)
	}

	thoughts, err := ParseThoughts(raw)
	if err != nil {
		t.Fatalf("parse production playbook: %v", err)
	}

	return thoughts
}

func collectActs(thoughts []Thought) []Act {
	var acts []Act

	var walk func([]Thought)
	walk = func(nodes []Thought) {
		for _, node := range nodes {
			if node.Do.Type != ActionNone {
				acts = append(acts, node.Do)
			}

			walk(node.Then)
		}
	}

	walk(thoughts)

	return acts
}

func signalSnapshot(
	category types.CategoryType, snr, confidence, last, volume float64,
) types.Measurement {
	return types.Measurement{
		Category:   category,
		SNR:        snr,
		Confidence: confidence,
		Last:       last,
		Volume:     volume,
	}
}

func priceTick(last, volume float64) types.Measurement {
	return types.Measurement{Last: last, Volume: volume}
}

func flatPosition() PositionState {
	return PositionState{}
}

func longStarted(entry, peak, last float64) PositionState {
	return PositionState{
		Holding:    true,
		Side:       trading.Buy,
		EntryPrice: entry,
		Peak:       peak,
		Last:       last,
	}
}

func playbookContext(
	regime types.Regime, position PositionState, snapshots []types.Measurement,
) *WindowReason {
	return NewWindowReason(snapshots, regime, position)
}

func evaluateProduction(
	playbook []Thought, ctx ReasonContext, state *ReasonState,
) (Act, bool) {
	return EvaluateStateful(playbook, ctx, state)
}

func buildFlashPumpSnapshots() []types.Measurement {
	prices := []float64{100, 100.02, 100.04, 100.06, 100.08, 100.10, 100.12, 100.14, 100.22}
	ignitionSeries := []float64{0.2, 0.4, 0.6, 0.8, 0.5, 0.7, 1.2}

	var snapshots []types.Measurement

	for _, price := range prices {
		snapshots = append(snapshots, priceTick(price, 1e6))
	}

	for range 3 {
		snapshots = append(snapshots, signalSnapshot(
			types.CategoryCoiledCompression, 0.5, 0.5, prices[len(prices)-1], 1e6,
		))
	}

	for _, snr := range ignitionSeries {
		snapshots = append(snapshots, signalSnapshot(
			types.CategoryVerticalIgnition, snr, 0, prices[len(prices)-1], 1e6,
		))
	}

	return snapshots
}

func buildMomentumParentSnapshots() []types.Measurement {
	return []types.Measurement{
		signalSnapshot(types.CategoryEndogenousAlpha, 1.3, 0, 101.6, 125),
		signalSnapshot(types.CategoryAggressiveDrive, 1.1, 0, 101.6, 125),
		signalSnapshot(types.CategoryLaminar, 0, 0.6, 101.6, 125),
	}
}

func buildMomentumConfirmSnapshots() []types.Measurement {
	var snapshots []types.Measurement

	for index := range 13 {
		price := 100 + float64(index)*0.04
		volume := 100 + float64(index)*0.85
		snapshots = append(snapshots, priceTick(price, volume))
	}

	snapshots = append(snapshots, buildMomentumParentSnapshots()...)

	return snapshots
}

func buildAbsorptionSnapshots(loadedNow, loadedAgo10 float64) []types.Measurement {
	var snapshots []types.Measurement

	loadedSeries := []float64{0.4, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1, 1.2, 1.1, 1.0, loadedAgo10, loadedNow}

	for _, snr := range loadedSeries {
		snapshots = append(snapshots, signalSnapshot(types.CategoryLoadedImbalance, snr, 0, 100, 1e6))
	}

	snapshots = append(snapshots,
		signalSnapshot(types.CategoryHiddenAbsorption, 0, 0.7, 100, 1e6),
		signalSnapshot(types.CategoryHardSupport, 0, 0.6, 100, 1e6),
		signalSnapshot(types.CategoryRobustLiquidity, 0, 0.55, 100, 1e6),
	)

	return snapshots
}

func buildDeadRegimeAuditSnapshots() []types.Measurement {
	var snapshots []types.Measurement

	collapseSeries := []float64{0.5, 0.8, 1.0, 1.2, 1.5, 1.8}

	for _, snr := range collapseSeries {
		snapshots = append(snapshots, signalSnapshot(
			types.CategoryMechanicalCollapse, snr, 0, 100, 1e6,
		))
	}

	snapshots = append(snapshots,
		signalSnapshot(types.CategoryDenseNeutrality, 1.2, 0, 100, 1e6),
		signalSnapshot(types.CategoryThermalExhaustion, 1.4, 0, 100, 1e6),
		signalSnapshot(types.CategoryEndogenousAlpha, 1.5, 0, 100, 1e6),
		signalSnapshot(types.CategoryAggressiveDrive, 1.2, 0, 100, 1e6),
		signalSnapshot(types.CategoryLaminar, 0, 0.7, 100, 1e6),
	)

	snapshots = append(snapshots, buildMomentumConfirmSnapshots()...)

	return snapshots
}

func TestProductionPlaybookContract(t *testing.T) {
	Convey("Given the production playbook loaded from perspectives.yaml", t, func() {
		playbook := loadProductionPlaybook(t)
		Convey("No node opens a short position", func() {
			for _, act := range collectActs(playbook) {
				So(IsShortAct(act), ShouldBeFalse)
			}
		})

		Convey("Dead regime flat blocks every entry even when momentum confirmation is live", func() {
			ctx := playbookContext(
				types.RegimeDead,
				flatPosition(),
				buildDeadRegimeAuditSnapshots(),
			)

			_, found := evaluateProduction(playbook, ctx, NewReasonState())
			So(found, ShouldBeFalse)
		})

		Convey("Choppy regime flat blocks entries with full momentum thesis", func() {
			ctx := playbookContext(
				types.RegimeChoppy,
				flatPosition(),
				buildMomentumConfirmSnapshots(),
			)

			_, found := evaluateProduction(playbook, ctx, NewReasonState())
			So(found, ShouldBeFalse)
		})

		Convey("Mechanical collapse denies the flash-pump parent even when ignition confirms", func() {
			snapshots := buildFlashPumpSnapshots()
			snapshots = append(snapshots, signalSnapshot(
				types.CategoryMechanicalCollapse, 1.2, 0, 100.85, 1e6,
			))

			ctx := playbookContext(types.RegimeTrending, flatPosition(), snapshots)
			_, found := evaluateProduction(playbook, ctx, NewReasonState())
			So(found, ShouldBeFalse)
		})

		Convey("Flash pump fires limit when coil, ignition edge, and price follow-through align", func() {
			ctx := playbookContext(
				types.RegimeTrending,
				flatPosition(),
				buildFlashPumpSnapshots(),
			)

			act, found := evaluateProduction(playbook, ctx, NewReasonState())
			So(found, ShouldBeTrue)
			So(act.Type, ShouldEqual, ActionLimit)
			So(IsShortAct(act), ShouldBeFalse)
		})

		Convey("Ignition below at_least does not fire flash pump", func() {
			snapshots := buildFlashPumpSnapshots()
			snapshots[len(snapshots)-1] = signalSnapshot(
				types.CategoryVerticalIgnition, 0.9, 0, 100.22, 1e6,
			)

			ctx := playbookContext(types.RegimeTrending, flatPosition(), snapshots)
			_, found := evaluateProduction(playbook, ctx, NewReasonState())
			So(found, ShouldBeFalse)
		})

		Convey("Momentum entry requires rising quote volume at confirmation", func() {
			parentOnly := append(
				buildMomentumParentSnapshots(),
				priceTick(101.2, 125),
			)

			ctx := playbookContext(types.RegimeBullish, flatPosition(), parentOnly)
			_, found := evaluateProduction(playbook, ctx, NewReasonState())
			So(found, ShouldBeFalse)
		})

		Convey("Momentum entry fires limit when volume and price rise at confirmation", func() {
			ctx := playbookContext(
				types.RegimeBullish,
				flatPosition(),
				buildMomentumConfirmSnapshots(),
			)

			act, found := evaluateProduction(playbook, ctx, NewReasonState())
			So(found, ShouldBeTrue)
			So(act.Type, ShouldEqual, ActionLimit)
		})

		Convey("Absorption entry requires loaded_imbalance at_least after crossed_up", func() {
			ctx := playbookContext(
				types.RegimeBullish,
				flatPosition(),
				buildAbsorptionSnapshots(1.1, 1.0),
			)

			_, found := evaluateProduction(playbook, ctx, NewReasonState())
			So(found, ShouldBeFalse)
		})

		Convey("Absorption entry fires limit when imbalance edge and level both hold", func() {
			ctx := playbookContext(
				types.RegimeBullish,
				flatPosition(),
				buildAbsorptionSnapshots(1.3, 1.0),
			)

			act, found := evaluateProduction(playbook, ctx, NewReasonState())
			So(found, ShouldBeTrue)
			So(act.Type, ShouldEqual, ActionLimit)
		})

		Convey("Scalp exit manager beats universal fallback on has_started", func() {
			snapshots := []types.Measurement{
				signalSnapshot(types.CategoryVerticalIgnition, 1.6, 0, 100, 1e6),
				signalSnapshot(types.CategoryCoiledCompression, 0.4, 0.55, 100, 1e6),
			}

			ctx := playbookContext(
				types.RegimeTrending,
				longStarted(100, 100, 100),
				snapshots,
			)

			act, found := evaluateProduction(playbook, ctx, NewReasonState())
			So(found, ShouldBeTrue)
			So(act.Type, ShouldEqual, ActionStopLoss)
			So(act.Offset, ShouldEqual, 0.008)
		})

		Convey("Universal fallback stop_loss protects a plain holding position", func() {
			ctx := playbookContext(types.RegimeDead, longStarted(100, 100, 100), nil)

			act, found := evaluateProduction(playbook, ctx, NewReasonState())
			So(found, ShouldBeTrue)
			So(act.Type, ShouldEqual, ActionStopLoss)
			So(act.Offset, ShouldEqual, 0.010)
		})

		Convey("Exit managers fire before entries when both sides would match", func() {
			snapshots := buildFlashPumpSnapshots()
			snapshots = append(snapshots,
				signalSnapshot(types.CategoryVerticalIgnition, 1.6, 0, 100.85, 1e6),
				signalSnapshot(types.CategoryCoiledCompression, 0.4, 0.55, 100.85, 1e6),
			)

			ctx := playbookContext(
				types.RegimeTrending,
				longStarted(100, 100, 100),
				snapshots,
			)

			act, found := evaluateProduction(playbook, ctx, NewReasonState())
			So(found, ShouldBeTrue)
			So(act.Type, ShouldEqual, ActionStopLoss)
			So(act.Type, ShouldNotEqual, ActionLimit)
		})

		Convey("Regime change clears a latched momentum parent before confirmation fires", func() {
			state := NewReasonState()
			parentCtx := playbookContext(
				types.RegimeTrending,
				flatPosition(),
				buildMomentumParentSnapshots(),
			)

			_, foundParent := evaluateProduction(playbook, parentCtx, state)
			So(foundParent, ShouldBeFalse)

			deadConfirm := playbookContext(
				types.RegimeDead,
				flatPosition(),
				buildMomentumConfirmSnapshots(),
			)

			_, foundDead := evaluateProduction(playbook, deadConfirm, state)
			So(foundDead, ShouldBeFalse)
		})

		Convey("A latched momentum parent fires limit once confirmation arrives in the same regime", func() {
			state := NewReasonState()
			parentCtx := playbookContext(
				types.RegimeBullish,
				flatPosition(),
				buildMomentumParentSnapshots(),
			)

			_, foundParent := evaluateProduction(playbook, parentCtx, state)
			So(foundParent, ShouldBeFalse)

			confirmCtx := playbookContext(
				types.RegimeBullish,
				flatPosition(),
				buildMomentumConfirmSnapshots(),
			)

			act, foundConfirm := evaluateProduction(playbook, confirmCtx, state)
			So(foundConfirm, ShouldBeTrue)
			So(act.Type, ShouldEqual, ActionLimit)
		})
	})
}

func TestProductionPlaybookContractStatefulSequence(t *testing.T) {
	Convey("Given flash pump parent and child split across ticks", t, func() {
		playbook := loadProductionPlaybook(t)
		state := NewReasonState()

		parentSnapshots := append(buildFlashPumpSnapshots()[:6],
			signalSnapshot(types.CategoryCoiledCompression, 0.5, 0.6, 100.3, 1e6),
			signalSnapshot(types.CategoryVerticalIgnition, 0.8, 0, 100.3, 1e6),
		)

		parentCtx := playbookContext(types.RegimeTrending, flatPosition(), parentSnapshots)
		_, foundParent := evaluateProduction(playbook, parentCtx, state)
		So(foundParent, ShouldBeFalse)

		confirmCtx := playbookContext(
			types.RegimeTrending,
			flatPosition(),
			buildFlashPumpSnapshots(),
		)

		act, foundConfirm := evaluateProduction(playbook, confirmCtx, state)
		So(foundConfirm, ShouldBeTrue)
		So(act.Type, ShouldEqual, ActionLimit)
	})
}

func TestProductionPlaybookContractExitOrdering(t *testing.T) {
	Convey("Given a held position past the universal time gate but still has_started", t, func() {
		playbook := loadProductionPlaybook(t)
		now := time.Now()
		position := PositionState{
			Holding:    true,
			Side:       trading.Buy,
			EntryPrice: 100,
			Peak:       100,
			Last:       100,
			EntryAt:    now.Add(-31 * time.Minute),
			Now:        now,
		}

		ctx := playbookContext(types.RegimeDead, position, nil)
		act, found := evaluateProduction(playbook, ctx, NewReasonState())

		So(found, ShouldBeTrue)
		So(act.Type, ShouldEqual, ActionStopLoss)
		So(act.Offset, ShouldEqual, 0.010)
	})
}

func BenchmarkProductionPlaybookEvaluate(b *testing.B) {
	raw, err := os.ReadFile(productionPlaybookPath(b))
	if err != nil {
		b.Fatal(err)
	}

	playbook, err := ParseThoughts(raw)
	if err != nil {
		b.Fatal(err)
	}

	ctx := playbookContext(
		types.RegimeTrending,
		flatPosition(),
		buildFlashPumpSnapshots(),
	)
	state := NewReasonState()

	for b.Loop() {
		state.Reset()
		_, _ = evaluateProduction(playbook, ctx, state)
	}
}
