package stack

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/types"
)

/*
CutCoordinator owns cut identity: it waits for signal results (or explicit
skips), freezes an ImmutableCut, and forwards the cut to Analyzer. Hawkes
remains the cadence trigger; every registered source must Report for that
CutID before finalize. Barrier state lives on the composed cutBarrier.
*/
type CutCoordinator struct {
	*types.Actor
	ctx      context.Context
	cancel   context.CancelFunc
	thesis   *types.Thesis
	counter  types.CutCounter
	tick     atomic.Int64
	barrier  cutBarrier
	recorder *audit.Recorder
}

/*
NewCutCoordinator constructs a coordinator for the listed signal sources.
*/
func NewCutCoordinator(
	ctx context.Context,
	thesis *types.Thesis,
	sources ...types.SourceType,
) *CutCoordinator {
	ctx, cancel := context.WithCancel(ctx)

	if len(sources) == 0 {
		sources = []types.SourceType{types.SourceHawkes}
	}

	coordinator := &CutCoordinator{
		ctx:     ctx,
		cancel:  cancel,
		thesis:  thesis,
		barrier: newCutBarrier(sources),
	}

	coordinator.Actor = types.NewActor(ctx, map[string]types.Handler{
		"ticker": {Topic: "ticker", Fn: coordinator.onHawkes},
		"trade":  {Topic: "trade", Fn: coordinator.onHawkes},
		"result": {Topic: "result", Fn: coordinator.onResult},
	})

	return coordinator
}

/*
SetRecorder attaches the runtime audit stream for cut-boundary phases.
*/
func (coordinator *CutCoordinator) SetRecorder(recorder *audit.Recorder) {
	coordinator.recorder = recorder
}

/*
Initialize attaches to Hawkes cadence topics with a normal buffer so cut frames
can queue while Analyzer drains depth-one, instead of reflecting cascade stalls
straight back into Hawkes measurement.
*/
func (coordinator *CutCoordinator) Initialize(hawkesActor *types.Actor) error {
	errnie.Info("initializing cut coordinator")

	coordinator.Actor.Initialize(
		types.Topic{Name: "ticker", Actor: hawkesActor},
		types.Topic{Name: "trade", Actor: hawkesActor},
	)

	return nil
}

/*
Report records one signal result for the active or specified cut.
*/
func (coordinator *CutCoordinator) Report(result types.SignalResult) {
	coordinator.barrier.Report(result)
}

/*
Close stops the coordinator Actor.
*/
func (coordinator *CutCoordinator) Close() error {
	coordinator.cancel()

	if coordinator.Actor != nil {
		return coordinator.Actor.Close()
	}

	return nil
}

func (coordinator *CutCoordinator) onResult(message any) any {
	result, ok := message.(types.SignalResult)

	if !ok {
		return nil
	}

	coordinator.Report(result)

	return nil
}

func (coordinator *CutCoordinator) onHawkes(message any) any {
	cutID := coordinator.begin(message)
	coordinator.barrier.autoSkip(cutID)
	coordinator.Report(types.SignalResult{
		CutID:  cutID,
		Source: types.SourceHawkes,
		Status: types.SignalReady,
	})

	if !coordinator.barrier.ready(cutID) {
		coordinator.barrier.fillMissing(cutID)
	}

	cut := coordinator.finalize(cutID)

	if cut != nil {
		errnie.Error(audit.Phase(coordinator.recorder, cut.Tick, "tick_begin", map[string]any{
			"cut_id": uint64(cut.ID),
		}))
		errnie.Error(audit.Phase(coordinator.recorder, cut.Tick, "measure_end", map[string]any{
			"measurements": len(cut.Measurements),
		}))
	}

	return &cutFrame{
		Cut:    cut,
		Thesis: coordinator.thesis,
		Hawkes: hawkesSource(message),
	}
}

func (coordinator *CutCoordinator) begin(message any) types.CutID {
	cutID := coordinator.counter.Next()
	coordinator.barrier.open(cutID)

	if thesis, ok := hawkesThesis(message); ok && thesis != nil {
		coordinator.thesis = thesis
	}

	return cutID
}

func (coordinator *CutCoordinator) finalize(cutID types.CutID) *types.ImmutableCut {
	tick := coordinator.tick.Add(1)
	at := time.Now().UTC()

	if coordinator.thesis != nil {
		coordinator.thesis.StampAt()

		if !coordinator.thesis.At.IsZero() {
			at = coordinator.thesis.At
		}

		// Cut identity only — do not ResetTick here. Signals already published
		// onto the durable thesis; wiping Forecasts/Manifold mid-cascade races
		// the analyzer/planner that still own this cut's outcomes.
		coordinator.thesis.Tick = tick
		coordinator.thesis.At = at
	}

	cut := types.NewImmutableCut(cutID, tick, coordinator.thesis)
	coordinator.barrier.clear(cutID)

	return cut
}

func hawkesThesis(message any) (*types.Thesis, bool) {
	switch value := message.(type) {
	case *hawkes.Cut:
		return value.SharedThesis(), true
	case *types.Thesis:
		return value, true
	default:
		return nil, false
	}
}

func hawkesSource(message any) manifold.HawkesSource {
	if source, ok := message.(manifold.HawkesSource); ok {
		return source
	}

	return nil
}

/*
cutFrame carries the immutable cut plus the durable thesis and frozen Hawkes
source so Analyzer bind stays a single message.
*/
type cutFrame struct {
	Cut    *types.ImmutableCut
	Thesis *types.Thesis
	Hawkes manifold.HawkesSource
}

func (frame *cutFrame) SharedThesis() *types.Thesis {
	if frame == nil {
		return nil
	}

	return frame.Thesis
}

func (frame *cutFrame) Symbols() []string {
	if frame == nil || frame.Hawkes == nil {
		return nil
	}

	return frame.Hawkes.Symbols()
}

func (frame *cutFrame) Outcome(symbol string) (excitation.Outcome, bool) {
	if frame == nil || frame.Hawkes == nil {
		return excitation.Outcome{}, false
	}

	return frame.Hawkes.Outcome(symbol)
}
