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
	reporter  *Reporter
	catalog   *tables.Catalog
	price     *broker.Price
	tee       runtime.Tee
	storeTee  runtime.Tee
}

func NewTraining(
	ctx context.Context,
	arena *data.ArenaOwner,
	price *broker.Price,
	catalog *tables.Catalog,
	tee runtime.Tee,
	storeTee runtime.Tee,
) *Training {
	engine := cognition.NewEngine(cognition.Config{})
	evaluator := NewEvaluator(price, engine)

	training := &Training{
		System:    runtime.NewSystem(ctx, "training", price),
		arena:     arena,
		grid:      store.NewGrid(),
		engine:    engine,
		evaluator: evaluator,
		detector:  NewDetector(ctx, storeTee),
		reporter:  NewReporter(data.NewArenaOwner(4096), tee),
		catalog:   catalog,
		price:     price,
		tee:       tee,
	}

	training.Transition(runtime.INIT)
	return training
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
	out.Tick = prior.Tick
	out.SeqIdx = prior.SeqIdx
	out.Label = prior.Label
	out.At = prior.At
	peers := prior.Peers

	if prior.Source != "runtime:join" {
		peers = []*data.Measurement[float64]{prior}
	}
	out.Peers = peers

	price := prior.GetMetric("price").Raw

	if price == 0 {
		for _, peer := range peers {
			if peerPrice := peer.GetMetric("price").Raw; peerPrice > 0 {
				price = peerPrice
				break
			}
		}
	}

	currentStatus := training.Status()

	if currentStatus == runtime.INIT {
		training.grid.Update(prior)

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
		}

		training.reporter.Populate(out, snapshot, nil)
		return out
	}

	return out
}

/*
Train drives Stage 1 historical training: scanning epoch market tape per symbol,
retrieving pre-split trajectories from Detector, and inserting them into the Radix trie.
*/
func (training *Training) Train() {
	go func() {
		if training == nil || training.catalog == nil {
			return
		}

		for training.Status() == runtime.INIT {
			select {
			case <-training.Context().Done():
				return
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}

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

				select {
				case <-training.Context().Done():
					return
				case <-time.After(1 * time.Second):
				}

				continue
			}

			slices.SortFunc(runs, func(left, right tables.Run) int {
				return cmp.Compare(left.Epoch, right.Epoch)
			})

			for _, run := range runs {
				select {
				case <-training.Context().Done():
					return
				default:
				}

				for detection := range training.catalog.Detections(training.Context(), run.Epoch) {
					select {
					case <-training.Context().Done():
						return
					default:
					}

					if detection == nil {
						continue
					}

					lowTick, highTick, tickErr := tables.DetectionTicks(detection)

					if tickErr != nil {
						training.Error(tickErr)
						continue
					}

					var (
						currentTick      int64 = -1
						tickMeasurements []*data.Measurement[float64]
						sequenceTokens   [][]byte
					)

					for measurement := range training.catalog.SignalLogic(
						training.Context(),
						detection.Epoch,
						detection.Label,
						lowTick,
						highTick,
					) {
						if measurement == nil {
							continue
						}

						if currentTick != -1 && measurement.Tick != currentTick {
							if len(tickMeasurements) > 0 {
								tokens := training.grid.LitRegions(tickMeasurements...)

								if len(tokens) > 0 {
									sequenceTokens = append(sequenceTokens, bytes.Join(tokens, []byte("_")))
								}

								tickMeasurements = tickMeasurements[:0]
							}
						}

						currentTick = measurement.Tick
						tickMeasurements = append(tickMeasurements, measurement)
					}

					if len(tickMeasurements) > 0 {
						tokens := training.grid.LitRegions(tickMeasurements...)

						if len(tokens) > 0 {
							sequenceTokens = append(sequenceTokens, bytes.Join(tokens, []byte("_")))
						}
					}

					if len(sequenceTokens) == 0 {
						continue
					}

					entryAsk, exitBid, priceErr := tables.DetectionPrices(detection)

					if priceErr != nil && training.price != nil {
						bid, ask := training.price.Touch(detection.Label)
						entryAsk = ask
						exitBid = bid
					}

					if entryAsk == nil || exitBid == nil || entryAsk.Sign() <= 0 || exitBid.Sign() <= 0 {
						continue
					}

					pnl, evalErr := training.evaluator.EvaluatePnL(detection.Label, entryAsk, exitBid)

					if evalErr != nil {
						training.Error(evalErr)
						continue
					}

					if len(sequenceTokens) > 2 {
						midPoint := len(sequenceTokens) / 2
						earlyPrefix := bytes.Join(sequenceTokens[:midPoint], []byte("/"))
						training.evaluator.ObserveWait(earlyPrefix)

						enterPrefix := bytes.Join(sequenceTokens[:midPoint+1], []byte("/"))
						training.evaluator.Train(enterPrefix, cognition.ActionEnter, pnl)

						exitPrefix := bytes.Join(sequenceTokens, []byte("/"))
						training.evaluator.Train(exitPrefix, cognition.ActionExit, pnl)
					}

					if len(sequenceTokens) <= 2 {
						fullSequence := bytes.Join(sequenceTokens, []byte("/"))
						training.evaluator.Train(fullSequence, cognition.ActionEnter, pnl)
					}
				}

				snap, snapErr := training.engine.Snapshot()

				if snapErr == nil && len(snap.Model) > 0 {
					training.catalog.PutBlob(
						training.Context(),
						fmt.Sprintf("trie/%d", run.Epoch),
						snap.Model,
					)
				}
			}

			select {
			case <-training.Context().Done():
				return
			case <-time.After(1 * time.Second):
			}
		}
	}()
}

/*
Detect finds stored trade tape from the measurements table that has not been scanned
for excursions yet. It should return only measurements with source = spot:trade,
group them by label, and sort them by epoch and tick.
*/
func (training *Training) Detect() {
	go func() {
		training.detector.Scan(
			training.catalog.Trades(training.Context()),
		)
	}()
}
