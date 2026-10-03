package strategy

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/ui"
)

var _ ui.CognitionSource = (*Training)(nil)

/*
Training develops one grid, checkpoints it with the trie, trains on historical
excursion trajectories, and starts paper trading once statistical edge is proven.
*/
type Training struct {
	*runtime.System
	arena     *data.ArenaOwner
	grid      *store.Grid
	engine    *cognition.Engine
	detector  *Detector
	evaluator *Evaluator
	skill     *Skill
	reporter  *Reporter
	trader    *Trader
	catalog   *tables.Catalog
	price     *broker.Price
	tee       runtime.Tee
	storeTee  runtime.Tee
}

func NewTraining(
	ctx context.Context,
	arena *data.ArenaOwner,
	price *broker.Price,
	trader *Trader,
	catalog *tables.Catalog,
	tee runtime.Tee,
	storeTee runtime.Tee,
) *Training {
	engine := cognition.NewEngine(cognition.Config{})
	evaluator := NewEvaluator(price, engine)

	training := &Training{
		System:    runtime.NewSystem(ctx, "strategy:training", price),
		arena:     arena,
		grid:      store.NewGrid(),
		engine:    engine,
		evaluator: evaluator,
		detector:  NewDetector(ctx, storeTee),
		skill:     NewSkill(),
		reporter:  NewReporter(data.NewArenaOwner(4096), tee),
		trader:    trader,
		catalog:   catalog,
		price:     price,
		tee:       tee,
	}

	training.Transition(runtime.INIT)

	if trader != nil {
		trader.OnPositionClosed(training.onPositionClosed)
	}

	return training
}

func (training *Training) onPositionClosed(symbol string, regulator *position.Regulator) {
	if training.price != nil && regulator != nil {
		pnl := training.price.ReturnPct(symbol, regulator)
		training.skill.RecordForward(pnl)
	}
}

func (training *Training) Arena() *data.ArenaOwner {
	return training.arena
}

func (training *Training) CognitionTree() cognition.CognitionTreeExport {
	if training == nil || training.engine == nil {
		return cognition.CognitionTreeExport{}
	}

	return training.engine.TreeExport()
}

/*
Step processes the live market signal across three operational stages:
 1. Grid development (Status: INIT): develops the grid until settled, then freezes and checkpoints it.
 2. Historical Validation (Status: WAITING): reports validation state while historical training runs.
 3. Live Paper Trading (Status: READY): matches live tokens against trained trie branches to trade paper positions.
*/
func (training *Training) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if prior == nil {
		return nil
	}

	out := training.arena.NewMeasurement(training.Name())
	out.Epoch = prior.Epoch
	out.SeqIdx = prior.SeqIdx
	out.Label = prior.Label
	out.At = prior.At
	peers := prior.Peers

	if prior.Source != "runtime:join" {
		peers = []*data.Measurement[float64]{prior}
	}
	out.Peers = peers

	price, _ := quotePrice(prior)
	currentStatus := training.Status()

	training.grid.Update(prior)
	defer training.grid.Update(out)

	if currentStatus == runtime.INIT {
		snapshot := ReportSnapshot{
			Source:  training.Name(),
			Symbol:  prior.Label,
			SeqIdx:  prior.SeqIdx,
			At:      prior.At,
			Stage:   StageModelDevelopment,
			Blocker: "grid developing",
			Price:   price,
		}
		training.reporter.Populate(out, snapshot, training.skill)

		if training.grid.IsSettled() {
			training.grid.Settle()
			snapshotData, err := training.grid.Snapshot()

			if err != nil {
				training.Error(errnie.Err(
					errnie.UnprocessableContent,
					fmt.Sprintf(
						"[training] unable to create snapshot of: grid/%d/%d",
						out.Epoch, out.SeqIdx,
					),
					err,
				))

				return out
			}

			training.catalog.PutBlob(
				training.Context(),
				fmt.Sprintf("grid/%d/%d", out.Epoch, out.SeqIdx),
				snapshotData,
			)

			training.Transition(runtime.WAITING)
		}

		return out
	}

	if currentStatus == runtime.WAITING {
		snapshot := ReportSnapshot{
			Source:  training.Name(),
			Symbol:  prior.Label,
			SeqIdx:  prior.SeqIdx,
			At:      prior.At,
			Stage:   StageHistoricalValidation,
			Blocker: training.skill.HistBlocker(),
			Price:   price,
		}
		training.reporter.Populate(out, snapshot, training.skill)

		if !training.skill.HasEdge() {
			return out
		}

		training.Transition(runtime.READY)
		return out
	}

	if currentStatus == runtime.READY {
		tokens := training.grid.LitRegions(append(peers, prior)...)

		stage := StageForwardPaperLearning
		blocker := training.skill.FwdBlocker()

		if training.skill.HasForwardEdge() {
			stage = StageForwardSkillValidated
			blocker = ""
		}

		snapshot := ReportSnapshot{
			Source:       training.Name(),
			Symbol:       prior.Label,
			SeqIdx:       prior.SeqIdx,
			At:           prior.At,
			Stage:        stage,
			Blocker:      blocker,
			RegionTokens: tokens,
			Price:        price,
			Trading:      true,
		}

		if len(tokens) > 0 {
			res, err := training.engine.Evaluate(bytes.Join(tokens, []byte("_")))

			if err != nil {
				training.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf(
						"[training] unable to evaluate: grid/%d/%d",
						out.Epoch, out.SeqIdx,
					),
					err,
				))

				training.reporter.Populate(out, snapshot, training.skill)
				return out
			}

			action := cognition.Action(res.Evaluation.WinnerClass)
			confidence := res.Evaluation.Confidence
			contrast := res.Evaluation.Surprisal

			err = training.trader.OnAction(
				prior.Label,
				action,
				confidence,
			)

			if err != nil {
				if broker.IsEnterSoftFail(err) {
					errnie.Error(err)
					training.reporter.Populate(out, snapshot, training.skill)
					return out
				}

				training.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf(
						"[training] failed to execute trader action: %d/%d",
						out.Epoch, out.SeqIdx,
					),
					err,
				))

				training.reporter.Populate(out, snapshot, training.skill)
				return out
			}

			actionCode := 0

			if action == cognition.ActionEnter {
				actionCode = 1
			}

			if action == cognition.ActionExit {
				actionCode = 2
			}

			snapshot.Action = actionCode
			snapshot.Confidence = confidence
			snapshot.Contrast = contrast
			training.skill.RecordPrediction()
		}

		training.reporter.Populate(out, snapshot, training.skill)
		return out
	}

	return out
}

/*
Run drives Stage 1 historical training: scanning epoch market tape per symbol,
retrieving pre-split trajectories from Detector, and inserting them into the Radix trie.
*/
func (training *Training) Run() {
	go func() {
		for training.Status() == runtime.INIT {
			select {
			case <-training.Context().Done():
				return
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}

		if training.catalog == nil {
			return
		}

		processedRuns := make(map[int64]bool)
		lastSeqProcessed := make(map[int64]int64)

		for {
			select {
			case <-training.Context().Done():
				return
			default:
			}

			runs, err := training.catalog.Runs(training.Context())

			if err != nil {
				training.Error(errnie.Err(
					errnie.BadGateway,
					"[training] failed to query runs from catalog",
					err,
				))
			}

			slices.SortFunc(runs, func(left, right tables.Run) int {
				return cmp.Compare(left.Epoch, right.Epoch)
			})

			allRunsProcessed := len(runs) > 0

			for _, run := range runs {
				if processedRuns[run.Epoch] {
					continue
				}

				allRunsProcessed = false
				maxSeq, hasEdge := training.processRun(run.Epoch, lastSeqProcessed[run.Epoch])
				lastSeqProcessed[run.Epoch] = maxSeq

				if maxSeq > 0 {
					processedRuns[run.Epoch] = true
				}

				if hasEdge {
					training.Transition(runtime.READY)
					return
				}
			}

			if training.skill.HasEdge() {
				training.Transition(runtime.READY)
				return
			}

			if allRunsProcessed && !training.skill.HasEdge() {
				processedRuns = make(map[int64]bool)
				lastSeqProcessed = make(map[int64]int64)
			}

			select {
			case <-training.Context().Done():
				return
			case <-time.After(1 * time.Second):
			}
		}
	}()
}

func (training *Training) processRun(epoch int64, lastSeq int64) (int64, bool) {
	currentLast := lastSeq

	labels, err := training.catalog.Labels(training.Context(), epoch)
	if err != nil || len(labels) == 0 {
		labels = []string{""}
	}

	maxSeq := currentLast

	for _, label := range labels {
		var timeline []*data.Measurement[float64]

		for measurement := range training.catalog.Timeline(
			training.Context(), epoch, label, currentLast+1, 0,
		) {
			if measurement == nil {
				continue
			}

			if measurement.SeqIdx <= currentLast {
				continue
			}

			if measurement.SeqIdx > maxSeq {
				maxSeq = measurement.SeqIdx
			}

			timeline = append(timeline, measurement)
		}

		if len(timeline) < 5 {
			continue
		}

		training.detector.Scan(timeline)

		for {
			trajectory, ok := training.detector.Next()
			if !ok {
				break
			}

			training.trainTrajectory(trajectory)

			if training.skill.HasEdge() {
				return maxSeq, true
			}
		}
	}

	return maxSeq, training.skill.HasEdge()
}

func (training *Training) trainTrajectory(trajectory Trajectory) {
	pnl, err := training.evaluator.EvaluatePnL(
		trajectory.Symbol,
		trajectory.EntryAsk,
		trajectory.ExitBid,
	)

	if err != nil {
		return
	}

	precursorTokens := slices.CompactFunc(
		training.grid.LitRegions(trajectory.Precursor...),
		bytes.Equal,
	)

	holdingTokens := slices.CompactFunc(
		training.grid.LitRegions(trajectory.Holding...),
		bytes.Equal,
	)

	if len(precursorTokens) > 0 {
		training.evaluator.Train(
			bytes.Join(precursorTokens, []byte("_")),
			cognition.ActionEnter,
			pnl,
		)
	}

	if len(holdingTokens) > 0 {
		training.evaluator.Train(
			bytes.Join(holdingTokens, []byte("_")),
			cognition.ActionExit,
			pnl,
		)
	}

	training.skill.RecordHistorical(pnl, pnl > 0)
	training.skill.RecordFragment("up")

	training.reporter.PublishExcursion(
		trajectory.Symbol,
		trajectory.Ticks[0],
		trajectory.Ticks[1],
		precursorTokens,
		pnl,
		training.skill,
	)
}
