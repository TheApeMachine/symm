package strategy

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

const (
	actionEnter = "enter"
	actionExit  = "exit"
	// actionWait names abstention / precursor stance. It is not a terminal
	// trie class — Teach rejects it; Recall with no enter/exit winner waits.
	actionWait = "wait"
)

/*
Training orchestrates TRAINING.md. Step is the pipeline stage: it develops
the Impulse Map until its regions settle, then (once Train has opened the
skill gate) paper trades the live market with the Model. Train rehearses the
stored excursions of past runs on the frozen grid in the background.

Training only wires its owners; each one owns its own state:

  - impulse: the grid, its checkpoint, and the per-symbol streams;
  - Model: the predictive trie;
  - Rehearsal: historical fragment learning and its UI fragments;
  - paper: live episodes, Desk actions, and realized refinement;
  - Reporter: the learning telemetry and its UI tee.
*/
type Training struct {
	*runtime.System
	Model     *Model
	Rehearsal *Rehearsal
	Reporter  *Reporter
	arena     *data.ArenaOwner
	impulse   *impulse
	paper     *paper
	detector  *Detector
	catalog   *tables.Catalog
	storeTee  runtime.Tee
	epoch     int64

	detectorDone chan struct{}
	scanOnce     sync.Once

	// passes counts finished Train runs for waiters.
	passes atomic.Int64
}

func NewTraining(
	ctx context.Context,
	arena *data.ArenaOwner,
	price *broker.Price,
	desk *broker.Desk,
	catalog *tables.Catalog,
	storeTee runtime.Tee,
	epoch int64,
) *Training {
	system := runtime.NewSystem(ctx, "training", price)
	model := NewModel()
	reporter := NewReporter()
	impulse := newImpulse(catalog)

	training := &Training{
		System: system,
		Model:  model,
		Rehearsal: newRehearsal(
			catalog, price, impulse, model, newChart(catalog, reporter, system.Name()), epoch,
		),
		Reporter:     reporter,
		arena:        arena,
		impulse:      impulse,
		detector:     NewDetector(ctx, storeTee, price),
		catalog:      catalog,
		storeTee:     storeTee,
		epoch:        epoch,
		detectorDone: make(chan struct{}),
	}

	// The detector labels every excursion against friction. Without it the
	// training system halts here instead of learning from mislabeled tape.
	if err := training.detector.Error(); err != nil {
		training.Error(errnie.Err(errnie.Internal, "[training] detector is not usable", err))
		return training
	}

	training.paper = newPaper(desk, model)
	training.Transition(runtime.INIT)
	return training
}

func (training *Training) Arena() *data.ArenaOwner {
	return training.arena
}

/*
Step executes the sequential development, paper trading, and reporting steps
for one measurement.
*/
func (training *Training) Step(prior *data.Measurement) *data.Measurement {
	if prior == nil {
		return nil
	}

	snapshot := ReportSnapshot{
		Source: training.Name(),
		Symbol: prior.Label,
		SeqIdx: prior.SeqIdx,
		At:     prior.At,
	}

	// Display only: sensory ticks that are not trades carry no price.
	// Absence leaves Price at 0 (omitted by the reporter); it never feeds
	// friction, profitability, or labels. A real read failure is logged —
	// never invent a price, and never treat it as absence.
	price, err := readMetric(prior, "price")

	if err != nil {
		errnie.Error(errnie.Err(errnie.IO, "[training] display price read failed", err))
	}

	if price != nil {
		snapshot.Price = price.Raw
	}

	status := training.Status()
	pass := training.impulse.observe(prior)

	if status == runtime.INIT {
		training.develop(prior, &snapshot)
	}

	snapshot.GridCells = training.impulse.grid.CellCount()
	snapshot.GridRegions = training.impulse.grid.RegionsFormed()

	if status == runtime.WAITING {
		snapshot.Stage = StageHistoricalValidation
		snapshot.Blocker = "loading trie from historical excursions"
	}

	if graded := training.Rehearsal.grade.Load(); status == runtime.WAITING && graded != nil {
		snapshot.Blocker = graded.blocker()
	}

	if status == runtime.READY {
		snapshot.Stage = StageForwardPaperLearning
		snapshot.Trading = true

		if token := training.impulse.grid.LitRegion(pass); len(token) > 0 {
			snapshot.RegionTokens = [][]byte{token}
			training.paper.trade(prior, token, &snapshot)
		}
	}

	snapshot.Resolved, snapshot.WinRate, snapshot.Edge = training.paper.score()

	peers := prior.Peers()

	if prior.Source != "runtime:join" {
		peers = []*data.Measurement{prior}
	}

	out := training.arena.NewMeasurement(
		prior.Epoch,
		prior.Label,
		training.Name(),
		prior.SeqIdx,
		prior.Tick,
		peers,
		training.Reporter.Metadata(snapshot)...,
	)
	out.At = prior.At
	out.From = prior.At

	return training.Reporter.Publish(out, snapshot)
}

/*
develop advances the Impulse Map and opens historical rehearsal (WAITING)
once the grid is settled and checkpointed.
*/
func (training *Training) develop(prior *data.Measurement, snapshot *ReportSnapshot) {
	snapshot.Stage = StageModelDevelopment
	snapshot.Blocker = "grid developing"

	settled, err := training.impulse.develop(training.Context(), prior.Epoch, prior.SeqIdx)

	if err != nil {
		// Internal closes training; root halts with this error.
		training.Error(errnie.Err(errnie.Internal, "[training] grid development failed", err))
		return
	}

	if settled {
		training.Transition(runtime.WAITING)
	}
}

func (training *Training) Passes() int64 {
	return training.passes.Load()
}

/*
Train waits for the grid to settle (region tokens are only comparable once it
is frozen) and for the detector scan of past runs, then rehearses every
stored excursion (Rehearsal.run). The best pass is then graded by the skill
gate; an open gate checkpoints the trie and opens paper trading (READY).
*/
func (training *Training) Train() {
	go func() {
		go training.runDetectorScan()

		for training.Status() == runtime.INIT {
			select {
			case <-training.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}

		select {
		case <-training.Context().Done():
			return
		case <-training.detectorDone:
		}

		if training.Status() != runtime.WAITING {
			return
		}

		defer training.passes.Add(1)

		graded, err := training.Rehearsal.run(training.Context())

		// A failed pass halts training. Internal closes the training
		// context, which cmd/root watches, so the process stops with this
		// error instead of opening paper trading on a trie that silently
		// missed part of its tape. The error is recorded before the pass is
		// counted so a waiter never observes a finished pass without it.
		if err != nil {
			training.Error(errnie.Err(errnie.Internal, "[training] failed during rehearsal", err))
			return
		}

		records, err := training.Model.Count("records")

		if err != nil {
			training.Error(errnie.Err(errnie.Internal, "[training] unable to read the trie census", err))
			return
		}

		if !graded.open() {
			errnie.Info(fmt.Sprintf("[training] paper trading stays closed: %s (%d records)", graded.blocker(), int(records)))
			return
		}

		// The checkpoint reads the in-memory trie. Failing to read it means
		// the cognition memory itself is broken, so training halts instead
		// of opening paper trading on it.
		model, err := training.Model.Checkpoint()

		if err != nil {
			training.Error(errnie.Err(errnie.Internal, "[training] unable to snapshot trie", err))
			return
		}

		// Object storage is optional (an unconfigured bucket is "no
		// checkpoint"), so a failed upload only loses the restart
		// checkpoint; the trie in memory is intact and correct.
		if err = training.catalog.PutBlob(
			training.Context(), fmt.Sprintf("trie/%d", graded.latest), model,
		); err != nil {
			errnie.Error(errnie.Err(errnie.IO, "[training] unable to checkpoint trie", err))
		}

		errnie.Info(fmt.Sprintf(
			"[training] trie loaded from %d of %d excursions (%d records)",
			graded.trained, graded.seen, int(records),
		))

		training.Transition(runtime.READY)
	}()
}

/*
runDetectorScan runs as its own background process.
It finds the latest stored detection (source = detector), and scans any new
trade tape collected for epochs strictly before the current run's epoch
(epoch < training.epoch). A run that already has stored detections is never
scanned again: a classless detection left over from before the excursion
classes halts a rehearsal pass, so the fix is to reset those tables, not to append
classified rows next to it. Once all prior tape before the current epoch is
exhausted, the process exits.

A catalog read failure halts training with an Internal error, because the
runs it hides would look like runs that need no scan, or like an empty tape.
*/
func (training *Training) runDetectorScan() {
	defer training.scanOnce.Do(func() {
		close(training.detectorDone)
	})

	if training.catalog == nil {
		return
	}

	ctx := training.Context()

	var (
		latestEpoch int64
		latestTick  int64
	)

	for det, err := range training.catalog.Detections(ctx) {
		// Shutdown cancels the read; that is not a storage failure.
		if err != nil && ctx.Err() != nil {
			return
		}

		if err != nil {
			training.Error(errnie.Err(
				errnie.Internal, "[training] unable to read detections for detector scan", err,
			))
			return
		}

		if det == nil || det.Epoch >= training.epoch {
			continue
		}

		if det.Epoch > latestEpoch || (det.Epoch == latestEpoch && det.Tick > latestTick) {
			latestEpoch = det.Epoch
			latestTick = det.Tick
		}
	}

	// Without the run list no prior tape can be scanned, and training would
	// rehearse only what happens to be stored already.
	runs, err := training.catalog.Runs(ctx)
	if err != nil {
		training.Error(errnie.Err(errnie.Internal, "[training] unable to list runs for detector scan", err))
		return
	}

	slices.SortFunc(runs, func(left, right tables.Run) int {
		return cmp.Compare(left.Epoch, right.Epoch)
	})

	for _, run := range runs {
		if run.Epoch >= training.epoch {
			continue
		}

		if latestEpoch > 0 && run.Epoch < latestEpoch {
			continue
		}

		stored := false

		for det, err := range training.catalog.Detections(ctx, run.Epoch) {
			if err != nil && ctx.Err() != nil {
				return
			}

			if err != nil {
				training.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("[training] unable to read detections for run %d", run.Epoch),
					err,
				))
				return
			}

			if det != nil {
				stored = true
				break
			}
		}

		if stored {
			continue
		}

		trades := training.catalog.Trades(ctx, run.Epoch)

		// A failed scan halts training: classifying the remaining tape
		// without friction would teach the trie mislabeled excursions, and
		// a tape whose read failed part-way would end its last excursion at
		// an arbitrary tick.
		if err := training.detector.Scan(trades); err != nil {
			training.Error(errnie.Err(errnie.Internal, "[training] detector scan failed", err))
			return
		}

		if training.storeTee != nil {
			writer := tables.NewWriter(training.catalog, run.Epoch)
			defer writer.ReleaseRemaining()
			drained := 0

			for {
				ptr := training.storeTee.Next()
				if ptr == nil {
					break
				}

				pub := data.To[data.Publication](ptr)
				if pub.Measurement == nil {
					pub.Release()
					continue
				}

				writer.Add(tables.Measurements, pub)
				drained++
			}

			// Detections that fail to commit never reach rehearsal, so the
			// trie would silently train without them.
			if drained > 0 {
				if commitErr := writer.CommitReady(ctx, true); commitErr != nil {
					training.Error(errnie.Err(
						errnie.Internal,
						fmt.Sprintf("[training] unable to commit detections for run %d", run.Epoch),
						commitErr,
					))
					return
				}
			}
		}
	}
}

/*
LearningSummary returns the latest high-density summary of the learning system.
*/
func (training *Training) LearningSummary() string {
	if training == nil || training.Reporter == nil {
		return ""
	}

	return training.Reporter.LatestSummary()
}

/*
LearningReport returns structured high-value report metrics for inspection.
*/
func (training *Training) LearningReport() any {
	if training == nil || training.Reporter == nil {
		return nil
	}

	return training.Reporter.ReportData()
}
