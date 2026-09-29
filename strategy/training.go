package strategy

import (
	"context"
	"fmt"
	"math"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/strategy/impulse"
	"github.com/theapemachine/symm/system"
)

type Action string

const (
	ActionEnter Action = "enter"
	ActionExit  Action = "exit"
	ActionWait  Action = "wait"
)

func LegalActions(holding bool) []Action {
	if !holding {
		return []Action{ActionEnter, ActionWait}
	}

	return []Action{ActionExit, ActionWait}
}

/*
Training composes the live owner-addressed impulse map with precursor encoding
and one shared radix trie. Complete historical tapes train the trie through
an independent map; the live path only evaluates current regions.
Training does not submit orders: quoted outcome evidence is not execution proof.
*/
type TeePusher interface {
	Push(*data.Measurement[float64])
}

type Training struct {
	*runtime.System
	measurement *data.Measurement[float64]
	precursor   *Precursor
	pipeline    *nomagique.Number
	engine      *cognition.Engine
	Rehearsal   *Rehearsal
	space       *impulse.Map
	trader      *Trader
	price       *broker.Price
	uiTee       TeePusher
	sequence    int64
}

func NewTraining(ctx context.Context, epoch int64, price *broker.Price) *Training {
	training := &Training{
		precursor: NewPrecursor(), space: impulse.NewMap(),
		engine: cognition.NewEngine(cognition.Config{MemoryScale: 1}),
		price:  price,
	}
	training.Rehearsal = NewRehearsal(ctx, epoch, price, training.engine)
	training.Rehearsal.SetOnProgress(training.publishProgress)
	training.pipeline = nomagique.NewNumber(training.space, training.precursor, training.engine)
	training.Register()
	training.System = runtime.NewSystem(ctx, "training")
	return training
}

func (training *Training) SetTee(tee TeePusher) {
	training.uiTee = tee
}

func (training *Training) SetTrader(trader *Trader) {
	training.trader = trader

	if trader != nil && training.precursor != nil {
		training.precursor.SetHolding(trader.Holding)
	}
}

/*
CognitionTree returns the live, real Radix Trie tree, active branches, and feasible actions
directly from the cognitive engine for visualization on the learning dashboard.
*/
func (training *Training) CognitionTree() cognition.CognitionTreeExport {
	if training == nil || training.engine == nil {
		return cognition.CognitionTreeExport{
			Root: &cognition.TrieNodeJSON{
				ID:          "root",
				TokenPrefix: "ROOT",
				Probability: 1.0,
				State:       "ESTIMATED",
			},
			Branches: []cognition.TrieBranchJSON{},
			Feasible: []cognition.FeasibleActionJSON{},
		}
	}

	return training.engine.TreeExport()
}

/*
Register initializes and returns the canonical training telemetry measurement
populated with all metrics required by the frontend dashboard.
*/
func (training *Training) Register() *data.Measurement[float64] {
	if training.measurement != nil {
		return training.measurement
	}

	training.measurement = data.NewMeasurement("training", map[string]data.Metric[float64]{
		"evaluated":       {Label: "evaluated"},
		"unsupported":     {Label: "unsupported"},
		"previous_input":  {Label: "previous_input"},
		"invalid_inputs":  {Label: "invalid_inputs"},
		"impulse_version": {Label: "impulse_version", Raw: grid.FormatVersion},
		"input_count":     data.NewMetric[float64]("input_count", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"steps":           data.NewMetric[float64]("steps", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"decisions":       data.NewMetric[float64]("decisions", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"resolved":        data.NewMetric[float64]("resolved", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"confidence":      data.NewMetric[float64]("confidence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"contrast":        data.NewMetric[float64]("contrast", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"surprisal":       data.NewMetric[float64]("surprisal", data.UnitNat, data.TimescaleInstantaneous, 0, 1),
		"ambiguity":       data.NewMetric[float64]("ambiguity", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"action":          data.NewMetric[float64]("action", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"win_rate":        data.NewMetric[float64]("win_rate", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"edge":            data.NewMetric[float64]("edge", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"progress":        data.NewMetric[float64]("progress", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"accuracy":        data.NewMetric[float64]("accuracy", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"support":         data.NewMetric[float64]("support", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"quality":         data.NewMetric[float64]("quality", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"trading":         data.NewMetric[float64]("trading", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"cash":            data.NewMetric[float64]("cash", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"equity":          data.NewMetric[float64]("equity", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"unrealized":      data.NewMetric[float64]("unrealized", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"positions":       data.NewMetric[float64]("positions", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"price":           data.NewMetric[float64]("price", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"excursion_type":  data.NewMetric[float64]("excursion_type", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"excursion_mag":   data.NewMetric[float64]("excursion_mag", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"mark_a":          data.NewMetric[float64]("mark_a", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"mark_b":          data.NewMetric[float64]("mark_b", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"mark_c":          data.NewMetric[float64]("mark_c", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"wins":            data.NewMetric[float64]("wins", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"losses":          data.NewMetric[float64]("losses", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"pnl":             data.NewMetric[float64]("pnl", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"agent_entry":     data.NewMetric[float64]("agent_entry", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"agent_exit":      data.NewMetric[float64]("agent_exit", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
	})

	training.measurement.Label = "learner"
	training.measurement.Metadata["peer-interest"] = "*"

	return training.measurement
}

/* Step evaluates the current owner-held metric publications in sequence order. */
func (training *Training) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if training.Status() != runtime.READY {
		errnie.Warn(training.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if measurement == nil {
		return nil
	}

	current := measurement

	if measurement.Source != "training" {
		current = training.measurement
	}

	if reading := training.Rehearsal.published.Load(); reading != nil {
		current.Metrics["decisions"] = current.Metrics["decisions"].Write(float64(reading.Learned))
		current.Metrics["resolved"] = current.Metrics["resolved"].Write(float64(reading.Resolved))
		current.Metrics["unsupported"] = current.Metrics["unsupported"].Write(float64(reading.Unsupported))
		current.Metrics["evaluated"] = current.Metrics["evaluated"].Write(float64(reading.Predicted))
		if reading.Entered > 0 {
			current.Metrics["edge"] = current.Metrics["edge"].Write(reading.Return / float64(reading.Entered))
			current.Metrics["win_rate"] = current.Metrics["win_rate"].Write(float64(reading.Profitable) / float64(reading.Entered))
			current.Metrics["wins"] = current.Metrics["wins"].Write(float64(reading.Profitable))
			current.Metrics["losses"] = current.Metrics["losses"].Write(float64(reading.Entered - reading.Profitable))
			current.Metrics["pnl"] = current.Metrics["pnl"].Write(reading.Return)
		}

		if reading.Predicted > 0 {
			current.Metrics["accuracy"] = current.Metrics["accuracy"].Write(float64(reading.Correct) / float64(reading.Predicted))
		}
	}

	current.Metrics["previous_input"] = current.Metrics["previous_input"].Write(float64(training.sequence))
	current.Metrics["input_count"] = current.Metrics["input_count"].Write(float64(len(measurement.Peers)))
	training.updatePortfolioMetrics(current)

	input := func(yield func(unsafe.Pointer) bool) { yield(unsafe.Pointer(measurement)) }

	for output := range training.pipeline.Next(input) {
		evaluation := *(*cognition.Evaluation)(output)
		current.Label = training.space.Current.Symbol
		current.Result = training.space.Current
		current.SeqIdx = measurement.SeqIdx
		current.At = training.space.Current.At
		current.Metrics["steps"] = current.Metrics["steps"].Write(current.Metrics["steps"].Raw + 1)
		current.Metrics["support"] = current.Metrics["support"].Write(float64(evaluation.Support))
		current.Metrics["confidence"] = current.Metrics["confidence"].Write(evaluation.Confidence)
		current.Metrics["contrast"] = current.Metrics["contrast"].Write(evaluation.Contrast)
		current.Metrics["surprisal"] = current.Metrics["surprisal"].Write(evaluation.Surprisal)
		current.Metrics["ambiguity"] = current.Metrics["ambiguity"].Write(evaluation.Ambiguity)
		action := Action(evaluation.WinnerClass)
		actionValue := 0.0

		if action == ActionEnter {
			actionValue = 1
			current.Metrics["agent_entry"] = current.Metrics["agent_entry"].Write(float64(measurement.SeqIdx))
		}

		if action == ActionExit {
			actionValue = 2
			current.Metrics["agent_exit"] = current.Metrics["agent_exit"].Write(float64(measurement.SeqIdx))
		}

		current.Metrics["action"] = current.Metrics["action"].Write(actionValue)

		authorized := true
		hasModelEdge := false

		if action == ActionEnter {
			minConf := system.UninformativeDirectionConfidence
			plannerConfig := system.NewPlannerConfig()

			if plannerConfig.CognitionSwitchConfidence > minConf {
				minConf = plannerConfig.CognitionSwitchConfidence
			}

			if reading := training.Rehearsal.published.Load(); reading != nil {
				if reading.Learned > 0 && reading.Return > 0 {
					if reading.Entered >= 10 {
						p := float64(reading.Profitable) / float64(reading.Entered)
						se := math.Sqrt(p * (1.0 - p) / float64(reading.Entered))
						hasModelEdge = (p - 0.5) > se
					}

					if reading.Entered < 10 {
						hasModelEdge = true
					}
				}
			}

			authorized = hasModelEdge &&
				evaluation.Support > 0 &&
				evaluation.Confidence > minConf &&
				!evaluation.IsBreak &&
				evaluation.Ambiguity < 0.85
		}

		if action == ActionEnter && !authorized && training.trader != nil {
			reason := fmt.Sprintf("authority gate rejected enter: support=%d, conf=%.3f, break=%v, amb=%.3f",
				evaluation.Support, evaluation.Confidence, evaluation.IsBreak, evaluation.Ambiguity)

			if reading := training.Rehearsal.published.Load(); reading == nil || reading.Learned == 0 {
				reason = "authority gate rejected enter: model is candidate/untrained (0 learned situations)"
			}

			if reading := training.Rehearsal.published.Load(); reading != nil && reading.Learned > 0 && !hasModelEdge {
				reason = "authority gate rejected enter: measured skill does not exceed uncertainty or return is non-positive"
			}

			training.trader.RecordDecision(
				current.Label,
				"unauthorized",
				evaluation.Confidence,
				reason,
			)
		}

		if training.trader != nil && ((action == ActionEnter && authorized) || action == ActionExit) {
			training.trader.OnAction(current.Label, action)
		}

		if training.trader != nil {
			actionName := "wait"

			if action == ActionEnter {
				actionName = "enter"
			}

			if action == ActionExit {
				actionName = "exit"
			}

			training.trader.RecordDecision(current.Label, actionName, evaluation.Confidence, "precursor evaluation")
		}
	}

	if err := training.pipeline.Error(); err != nil {
		current.Err = err
		training.Error(err)
		training.Transition(runtime.ERROR)
	}

	if training.trader != nil {
		training.updatePortfolioMetrics(current)
	}

	priceSource := training.price

	if priceSource == nil && training.trader != nil {
		priceSource = training.trader.price
	}

	targetSymbol := current.Label

	if targetSymbol == "" {
		targetSymbol = measurement.Label
	}

	if priceSource != nil && targetSymbol != "" {
		if mark := priceSource.CurrentMark(targetSymbol); mark != nil {
			current.Metrics["price"] = current.Metrics["price"].Write(mark.Float64())
		}
	}

	if training.Rehearsal != nil {
		if lastRecord := training.Rehearsal.LastExcursion(); lastRecord != nil {
			extType := 0.0

			if lastRecord.Direction == "upward" {
				extType = 1.0
			}

			if lastRecord.Direction == "downward" {
				extType = 2.0
			}

			current.Metrics["excursion_type"] = current.Metrics["excursion_type"].Write(extType)
			current.Metrics["excursion_mag"] = current.Metrics["excursion_mag"].Write(lastRecord.GrossExcursion * 100)
			current.Metrics["mark_a"] = current.Metrics["mark_a"].Write(float64(lastRecord.PrecursorStartTick))
			current.Metrics["mark_b"] = current.Metrics["mark_b"].Write(float64(lastRecord.AnchorTick))
			current.Metrics["mark_c"] = current.Metrics["mark_c"].Write(float64(lastRecord.ExitTick))
		}
	}

	current.Metrics["invalid_inputs"] = current.Metrics["invalid_inputs"].Write(float64(training.space.Invalid))
	training.sequence = measurement.SeqIdx
	training.measurement = current
	return current
}

func (training *Training) updatePortfolioMetrics(current *data.Measurement[float64]) {
	if training == nil || training.trader == nil || current == nil {
		return
	}

	positions := training.trader.PositionCount()
	current.Metrics["positions"] = current.Metrics["positions"].Write(float64(positions))

	tradingVal := 0.0

	if positions > 0 {
		tradingVal = 1.0
	}

	current.Metrics["trading"] = current.Metrics["trading"].Write(tradingVal)

	balance := training.trader.Balance()

	if balance == nil {
		return
	}

	if cash := balance.Cash(); cash != nil {
		current.Metrics["cash"] = current.Metrics["cash"].Write(cash.Float64())
	}

	if equity := balance.Equity(); equity != nil {
		current.Metrics["equity"] = current.Metrics["equity"].Write(equity.Float64())
	}

	if unrealized := balance.Unrealized(); unrealized != nil {
		current.Metrics["unrealized"] = current.Metrics["unrealized"].Write(unrealized.Float64())
	}
}

func (training *Training) publishProgress(frame *data.Measurement[float64], record *tables.ExcursionRecord, impulse *grid.Snapshot) {
	if training == nil || frame == nil {
		return
	}

	prototype := training.Register()

	if prototype == nil {
		return
	}

	current := prototype.Clone()
	training.updatePortfolioMetrics(current)
	reading := training.Rehearsal.published.Load()

	if reading != nil {
		current.Metrics["decisions"] = current.Metrics["decisions"].Write(float64(reading.Learned))
		current.Metrics["resolved"] = current.Metrics["resolved"].Write(float64(reading.Resolved))
		current.Metrics["unsupported"] = current.Metrics["unsupported"].Write(float64(reading.Unsupported))
		current.Metrics["evaluated"] = current.Metrics["evaluated"].Write(float64(reading.Predicted))

		if reading.Entered > 0 {
			current.Metrics["edge"] = current.Metrics["edge"].Write(reading.Return / float64(reading.Entered))
			current.Metrics["win_rate"] = current.Metrics["win_rate"].Write(float64(reading.Profitable) / float64(reading.Entered))
			current.Metrics["wins"] = current.Metrics["wins"].Write(float64(reading.Profitable))
			current.Metrics["losses"] = current.Metrics["losses"].Write(float64(reading.Entered - reading.Profitable))
			current.Metrics["pnl"] = current.Metrics["pnl"].Write(reading.Return)
		}

		if reading.Predicted > 0 {
			current.Metrics["accuracy"] = current.Metrics["accuracy"].Write(float64(reading.Correct) / float64(reading.Predicted))
		}
	}

	current.SeqIdx = frame.SeqIdx
	current.At = frame.At
	current.Label = frame.Label

	if record != nil && record.Symbol != "" {
		current.Label = record.Symbol
	}

	if impulse != nil && impulse.Label != "" {
		current.Label = impulse.Label
	}

	if current.Label == "" || current.Label == "replay" {
		errnie.Error(errnie.Err(errnie.Validation, "training: progress frame has no valid symbol label", nil))
		return
	}

	if frame.Metrics != nil {
		if priceMetric, ok := frame.Metrics["price"]; ok && priceMetric.Raw > 0 {
			current.Metrics["price"] = current.Metrics["price"].Write(priceMetric.Raw)
		}

		if askMetric, ok := frame.Metrics["ask"]; ok && askMetric.Raw > 0 {
			if bidMetric, ok := frame.Metrics["bid"]; ok && bidMetric.Raw > 0 {
				current.Metrics["price"] = current.Metrics["price"].Write((askMetric.Raw + bidMetric.Raw) / 2)
			}
		}
	}

	if record != nil && record.EntryPrice > 0 {
		current.Metrics["price"] = current.Metrics["price"].Write(record.EntryPrice)
	}

	if record != nil {
		extType := 0.0

		if record.Direction == "upward" {
			extType = 1.0
		}

		if record.Direction == "downward" {
			extType = 2.0
		}

		current.Metrics["excursion_type"] = current.Metrics["excursion_type"].Write(extType)
		current.Metrics["excursion_mag"] = current.Metrics["excursion_mag"].Write(record.GrossExcursion * 100)
		current.Metrics["mark_a"] = current.Metrics["mark_a"].Write(float64(record.PrecursorStartTick))
		current.Metrics["mark_b"] = current.Metrics["mark_b"].Write(float64(record.AnchorTick))
		current.Metrics["mark_c"] = current.Metrics["mark_c"].Write(float64(record.ExitTick))

		actionVal := 0.0

		if record.ClearsFriction {
			actionVal = 1.0
			current.Metrics["agent_entry"] = current.Metrics["agent_entry"].Write(float64(record.AnchorTick))
			current.Metrics["agent_exit"] = current.Metrics["agent_exit"].Write(float64(record.ExitTick))
		}

		current.Metrics["action"] = current.Metrics["action"].Write(actionVal)
	}

	if impulse != nil {
		current.Result = impulse
	}

	if training.uiTee != nil {
		training.uiTee.Push(current)
	}
}
