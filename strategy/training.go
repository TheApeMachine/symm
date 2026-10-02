package strategy

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/samber/lo"
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

const (
	gridKey   = "model/grid.json"
	engineKey = "model/cognition.gob"
	skillKey  = "model/skill.json"
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
}

func NewTraining(
	ctx context.Context,
	arena *data.ArenaOwner,
	price *broker.Price,
	trader *Trader,
	catalog *tables.Catalog,
	tee runtime.Tee,
) *Training {
	engine := cognition.NewEngine(cognition.Config{})
	evaluator := NewEvaluator(price, engine)

	training := &Training{
		System:    runtime.NewSystem(ctx, "training", price),
		arena:     arena,
		grid:      store.NewGrid(),
		engine:    engine,
		evaluator: evaluator,
		detector:  NewDetector(),
		skill:     NewSkill(),
		reporter:  NewReporter(arena, tee),
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

func (training *Training) onPositionClosed(symbol string, reg *position.Regulator) {
	if training.price != nil && reg != nil {
		training.skill.Record(training.price.ReturnPct(symbol, reg))
	}
}

func (training *Training) Source() string {
	return "training"
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
Step processes the live market signal across two stages:
 1. Grid development (Status: INIT): develops the grid until settled, then freezes and checkpoints it.
 2. Live Paper Trading (Status: READY): matches live tokens against trained trie branches to trade paper positions.
*/
func (training *Training) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if prior == nil {
		return nil
	}

	out := training.arena.NewMeasurement(training.Source())
	out.Epoch = prior.Epoch
	out.SeqIdx = prior.SeqIdx
	out.Label = prior.Label
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	price, _ := quotePrice(prior)
	currentStatus := training.Status()

	if currentStatus == runtime.INIT {
		training.grid.Update(prior)

		training.reporter.Publish(ReportSnapshot{
			Source:  "training:live",
			Symbol:  prior.Label,
			SeqIdx:  prior.SeqIdx,
			At:      prior.At,
			Stage:   StageModelDevelopment,
			Blocker: "grid developing",
			Price:   price,
		}, training.skill)

		if training.grid.IsSettled() {
			training.grid.Settle()
			snapshot, err := training.grid.Snapshot()

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
				snapshot,
			)

			training.Transition(runtime.WAITING)
		}

		return out
	}

	if currentStatus == runtime.WAITING {
		if !training.skill.HasEdge() {
			return out
		}

		training.Transition(runtime.READY)

		// Note: out is the Step method's return Measurement value, which will already
		// be streamed to the frontend via the uiTee integrated in the LMAX Disruptor
		// pipeline. So just make sure your out Measurement has the things set you
		// need at each stage to make the UI show what it needs to show, and simply
		// read out the Measurement at the frontend. No need to invent all kinds
		// of new presentation types.
		return out
	}

	if training.Status() == runtime.READY {
		// The prior Measurement should always contain the prior sequence of
		// region tokens, when applicable.
		tokens := training.grid.LitRegions(append(prior.Peers, prior)...)

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

				return out
			}

			err = training.trader.OnAction(
				prior.Label,
				cognition.Action(res.Evaluation.WinnerClass),
				res.Evaluation.Confidence,
			)

			if err != nil {
				training.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf(
						"[training] failed to execute trader action: %d/%d",
						out.Epoch, out.SeqIdx,
					),
					err,
				))

				return out
			}

			out.WriteMetric("confidence", res.Evaluation.Confidence)
			out.WriteMetric("contrast", res.Evaluation.Surprisal)
			return out
		}
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

		// TODO: Need to actually query with partition awareness, which will
		// increase retrieval performance.
		measurements, err := training.catalog.Collect(
			training.Context(), tables.Measurements, 0,
		)

		if err != nil {
			training.Error(errnie.Err(
				errnie.Internal,
				"[training] failed to collect measurements from catalog",
				err,
			))

			return
		}

		if len(measurements) == 0 {
			return
		}

		for {
			select {
			case <-training.Context().Done():
				return
			default:
			}

			// Group measurements by Source and Symbol.
			grouped := lo.GroupBy(measurements, func(
				m *data.Measurement[float64],
			) struct{ Source, Symbol string } {
				return struct{ Source, Symbol string }{
					Source: m.Source,
					Symbol: m.Label,
				}
			})

			for groupKey, groupValue := range grouped {
				if groupKey.Source != "spot:ticker" {
					continue
				}

				training.detector.Scan(groupValue)

				for {
					chunks := training.detector.Next()

					if len(chunks[0]) == 0 {
						break
					}

					precursor := slices.CompactFunc(
						training.grid.LitRegions(chunks[0]...),
						bytes.Equal,
					)

					holding := slices.CompactFunc(
						training.grid.LitRegions(chunks[1]...),
						bytes.Equal,
					)

					entryMeas := chunks[0][len(chunks[0])-1]
					exitMeas := chunks[1][len(chunks[1])-1]

					entryAsk := entryMeas.GetMetric("ask").Exact
					exitBid := exitMeas.GetMetric("bid").Exact

					if entryAsk == nil || exitBid == nil || entryAsk.Sign() <= 0 || exitBid.Sign() <= 0 {
						continue
					}

					entryCost := training.price.WithFee(groupKey.Symbol, entryAsk, broker.BUY)
					exitNet := training.price.WithFee(groupKey.Symbol, exitBid, broker.SELL)

					if entryCost == nil || exitNet == nil || entryCost.Sign() <= 0 {
						continue
					}

					pnl := exitNet.Sub(entryCost).Div(entryCost).Float64()

					if len(precursor) > 0 {
						training.evaluator.Train(bytes.Join(
							precursor, []byte("_"),
						), cognition.ActionEnter, pnl)
					}
					if len(holding) > 0 {
						training.evaluator.Train(bytes.Join(
							holding, []byte("_"),
						), cognition.ActionExit, pnl)
					}

					training.skill.Record(pnl)
				}
			}

			if training.skill.HasEdge() {
				training.Transition(runtime.READY)
				return
			}
		}
	}()
}
