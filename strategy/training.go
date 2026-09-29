package strategy

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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

type Stage string

const (
	StageModelDevelopment         Stage = "MODEL DEVELOPMENT"
	StageHistoricalValidation     Stage = "HELD-OUT HISTORICAL VALIDATION"
	StageForwardPaperLearning     Stage = "FORWARD PAPER LEARNING"
	StageForwardSkillDemonstrated Stage = "FORWARD SKILL DEMONSTRATED"
)

type ForwardPrediction struct {
	Symbol     string
	ContextKey []byte
	Tokens     []uint64
	Tick       int64
	Action     Action
	Support    uint64
	Confidence float64
	Ambiguity  float64
}

type ForwardPaperReading struct {
	EnterPredictions   uint64
	CorrectEnter       uint64
	MissedEnter        uint64
	FalseEnterFriction uint64
	FalseEnterDown     uint64
	FalseEnterChop     uint64
	FalseEnterFlat     uint64
	CorrectWaitDown    uint64
	CorrectWaitChop    uint64
	CorrectWaitFlat    uint64
	ExitPredictions    uint64
	CorrectExit        uint64
	PrematureExit      uint64
	MissedExit         uint64

	PaperTrades     uint64
	PaperProfitable uint64
	PaperLosing     uint64
	PaperReturn     float64
	PaperReturnSq   float64
	PaperMeanReturn float64
	PaperReturnSE   float64
	PaperLowerBound float64
	PaperFees       float64
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
	measurement        *data.Measurement[float64]
	prototype          *data.Measurement[float64]
	paths              map[string]*data.Measurement[float64]
	precursor          *Precursor
	pipeline           *nomagique.Number
	engine             *cognition.Engine
	Rehearsal          *Rehearsal
	space              *impulse.Map
	detector           *tables.StreamingDetector
	trader             *Trader
	price              *broker.Price
	uiTee              TeePusher
	sequence           atomic.Int64
	stage              Stage
	stageBlocker       string
	stageMu            sync.RWMutex
	forwardPredictions map[string]ForwardPrediction
	forwardMu          sync.Mutex
	forwardReading     ForwardPaperReading
	focusSymbol        string
}

func NewTraining(ctx context.Context, epoch int64, price *broker.Price) *Training {
	training := &Training{
		precursor:          NewPrecursor(),
		space:              impulse.NewMap(),
		engine:             cognition.NewEngine(cognition.Config{MemoryScale: 1}),
		price:              price,
		detector:           tables.NewStreamingDetector(epoch, price),
		stage:              StageModelDevelopment,
		stageBlocker:       "waiting for historical tape replay",
		forwardPredictions: make(map[string]ForwardPrediction),
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

	if trader != nil {
		if training.precursor != nil {
			training.precursor.SetHolding(trader.Holding)
		}

		trader.SetOnPositionClosed(training.RecordForwardPaperTrade)
	}
}

func (training *Training) Stage() (Stage, string) {
	training.stageMu.RLock()
	defer training.stageMu.RUnlock()
	return training.stage, training.stageBlocker
}

func (training *Training) SetStage(stage Stage, blocker string) {
	training.stageMu.Lock()
	defer training.stageMu.Unlock()
	training.stage = stage
	training.stageBlocker = blocker
}

func (training *Training) SetFocusSymbol(symbol string) {
	training.stageMu.Lock()
	defer training.stageMu.Unlock()
	training.focusSymbol = symbol
}

func (training *Training) FocusSymbol() string {
	training.stageMu.RLock()
	defer training.stageMu.RUnlock()
	return training.focusSymbol
}

func (training *Training) ForwardReading() ForwardPaperReading {
	training.stageMu.RLock()
	defer training.stageMu.RUnlock()
	return training.forwardReading
}

func (training *Training) RecordForwardPaperTrade(symbol string, returnFraction float64, fee float64) {
	training.stageMu.Lock()
	defer training.stageMu.Unlock()

	training.forwardReading.PaperTrades++
	training.forwardReading.PaperReturn += returnFraction
	training.forwardReading.PaperReturnSq += returnFraction * returnFraction
	training.forwardReading.PaperFees += fee

	if returnFraction > 0 {
		training.forwardReading.PaperProfitable++
	}

	if returnFraction < 0 {
		training.forwardReading.PaperLosing++
	}

	if training.forwardReading.PaperTrades >= 2 {
		count := float64(training.forwardReading.PaperTrades)
		mean := training.forwardReading.PaperReturn / count
		variance := (training.forwardReading.PaperReturnSq - (training.forwardReading.PaperReturn*training.forwardReading.PaperReturn)/count) / (count - 1.0)

		if variance < 0 {
			variance = 0
		}

		se := math.Sqrt(variance / count)
		training.forwardReading.PaperMeanReturn = mean
		training.forwardReading.PaperReturnSE = se
		training.forwardReading.PaperLowerBound = mean - se
	}
}

func (training *Training) CheckStageGates() {
	training.stageMu.Lock()
	defer training.stageMu.Unlock()

	var reading *trainingReading

	if training.Rehearsal != nil {
		reading = training.Rehearsal.published.Load()

		if reading == nil {
			training.Rehearsal.readingMu.RLock()
			copied := training.Rehearsal.reading
			training.Rehearsal.readingMu.RUnlock()
			reading = &copied
		}
	}

	if reading == nil {
		training.stage = StageModelDevelopment
		training.stageBlocker = "waiting for historical tape replay"
		return
	}

	if training.stage == StageModelDevelopment {
		if reading.Resolved > 0 {
			training.stage = StageHistoricalValidation
			training.stageBlocker = "evaluating historical prequential skill"
		}

		if reading.Resolved == 0 {
			training.stageBlocker = "replaying historical tape; no resolved fragments yet"
			return
		}
	}

	if training.stage == StageHistoricalValidation {
		hasEntryEvidence := reading.ValidUpOpportunities > 0 && reading.Entered >= 2
		hasExitEvidence := reading.ContinuationWaitCorrect > 0 || reading.CorrectExit > 0
		hasEconomicUncertainty := reading.ReturnSE > 0
		hasPositiveLowerBound := reading.LowerBound > 0

		if !hasEntryEvidence {
			training.stageBlocker = "insufficient historical entry evidence (need >= 2 entered)"
			return
		}

		if !hasExitEvidence {
			training.stageBlocker = "waiting for valid historical exit evidence"
			return
		}

		if !hasEconomicUncertainty {
			training.stageBlocker = "historical economic uncertainty undefined"
			return
		}

		if !hasPositiveLowerBound {
			training.stageBlocker = fmt.Sprintf("historical return uncertainty spans zero (mean=%.4f, SE=%.4f, lower=%.4f)",
				reading.MeanReturn, reading.ReturnSE, reading.LowerBound)
			return
		}

		training.stage = StageForwardPaperLearning
		training.stageBlocker = ""
	}

	if training.stage == StageForwardPaperLearning {
		hasPaperTrades := training.forwardReading.PaperTrades >= 2
		hasPaperUncertainty := training.forwardReading.PaperReturnSE > 0
		hasPaperPositiveLowerBound := training.forwardReading.PaperLowerBound > 0
		hasNoNegativeClassFalseEnters := training.forwardReading.FalseEnterDown == 0 &&
			training.forwardReading.FalseEnterChop == 0 &&
			training.forwardReading.FalseEnterFlat == 0

		if !hasPaperTrades {
			training.stageBlocker = "no forward paper round trips completed (need >= 2)"
			return
		}

		if !hasPaperUncertainty {
			training.stageBlocker = "forward paper return uncertainty undefined"
			return
		}

		if !hasPaperPositiveLowerBound {
			training.stageBlocker = fmt.Sprintf("forward paper return uncertainty spans zero (mean=%.4f, SE=%.4f, lower=%.4f)",
				training.forwardReading.PaperMeanReturn, training.forwardReading.PaperReturnSE, training.forwardReading.PaperLowerBound)
			return
		}

		if !hasNoNegativeClassFalseEnters {
			training.stageBlocker = "forward model admitted false entries on negative tape"
			return
		}

		training.stage = StageForwardSkillDemonstrated
		training.stageBlocker = ""
	}
}

func (training *Training) freezeForwardPrediction(symbol string, contextKey []byte, tokens []uint64, tick int64, action Action, eval cognition.Evaluation) {
	training.forwardMu.Lock()
	defer training.forwardMu.Unlock()

	if training.forwardPredictions == nil {
		training.forwardPredictions = make(map[string]ForwardPrediction)
	}

	training.forwardPredictions[symbol] = ForwardPrediction{
		Symbol:     symbol,
		ContextKey: slices.Clone(contextKey),
		Tokens:     slices.Clone(tokens),
		Tick:       tick,
		Action:     action,
		Support:    eval.Support,
		Confidence: eval.Confidence,
		Ambiguity:  eval.Ambiguity,
	}
}

func (training *Training) resolveForwardOutcome(record *tables.ExcursionRecord) {
	if record == nil {
		return
	}

	training.forwardMu.Lock()
	frozen, found := training.forwardPredictions[record.Symbol]

	if found {
		delete(training.forwardPredictions, record.Symbol)
	}

	training.forwardMu.Unlock()

	record.Direction = strings.ToUpper(record.Direction)
	isUsefulUp := record.Direction == "UP" && record.ClearsFriction && record.ProfitFraction > 0

	if found {
		if frozen.Action == ActionEnter {
			training.forwardReading.EnterPredictions++

			if isUsefulUp {
				training.forwardReading.CorrectEnter++
			}

			if !isUsefulUp {
				switch record.Direction {
				case "UP":
					training.forwardReading.FalseEnterFriction++
				case "DOWN":
					training.forwardReading.FalseEnterDown++
				case "CHOP":
					training.forwardReading.FalseEnterChop++
				case "FLAT":
					training.forwardReading.FalseEnterFlat++
				}
			}
		}

		if frozen.Action == ActionWait {
			if isUsefulUp {
				training.forwardReading.MissedEnter++
			}

			if !isUsefulUp {
				switch record.Direction {
				case "DOWN":
					training.forwardReading.CorrectWaitDown++
				case "CHOP":
					training.forwardReading.CorrectWaitChop++
				case "FLAT":
					training.forwardReading.CorrectWaitFlat++
				}
			}
		}

		expectedAction := ActionWait

		if isUsefulUp {
			expectedAction = ActionEnter
		}

		training.engine.Observe(cognition.Association{
			Context: frozen.ContextKey,
			Class:   []byte(expectedAction),
			Graded:  false,
		})
	}

	training.CheckStageGates()
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
		"evaluated":             {Label: "evaluated"},
		"unsupported":           {Label: "unsupported"},
		"previous_input":        {Label: "previous_input"},
		"invalid_inputs":        {Label: "invalid_inputs"},
		"impulse_version":       {Label: "impulse_version", Raw: grid.FormatVersion},
		"input_count":           data.NewMetric[float64]("input_count", data.UnitCount, data.TimescaleInstantaneous, 0, 0),
		"steps":                 data.NewMetric[float64]("steps", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"decisions":             data.NewMetric[float64]("decisions", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"resolved":              data.NewMetric[float64]("resolved", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"confidence":            data.NewMetric[float64]("confidence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"contrast":              data.NewMetric[float64]("contrast", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"surprisal":             data.NewMetric[float64]("surprisal", data.UnitNat, data.TimescaleInstantaneous, 0, 1),
		"ambiguity":             data.NewMetric[float64]("ambiguity", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"action":                data.NewMetric[float64]("action", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"win_rate":              data.NewMetric[float64]("win_rate", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"edge":                  data.NewMetric[float64]("edge", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"progress":              data.NewMetric[float64]("progress", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"accuracy":              data.NewMetric[float64]("accuracy", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"support":               data.NewMetric[float64]("support", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"quality":               data.NewMetric[float64]("quality", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"trading":               data.NewMetric[float64]("trading", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"price":                 data.NewMetric[float64]("price", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"excursion_type":        data.NewMetric[float64]("excursion_type", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"excursion_mag":         data.NewMetric[float64]("excursion_mag", data.UnitPercent, data.TimescaleInstantaneous, 0, 1),
		"mark_a":                data.NewMetric[float64]("mark_a", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"mark_b":                data.NewMetric[float64]("mark_b", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"mark_c":                data.NewMetric[float64]("mark_c", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"wins":                  data.NewMetric[float64]("wins", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"losses":                data.NewMetric[float64]("losses", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"pnl":                   data.NewMetric[float64]("pnl", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"agent_entry":           data.NewMetric[float64]("agent_entry", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"agent_exit":            data.NewMetric[float64]("agent_exit", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"stage_code":            data.NewMetric[float64]("stage_code", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"fragments_up":          data.NewMetric[float64]("fragments_up", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"fragments_down":        data.NewMetric[float64]("fragments_down", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"fragments_chop":        data.NewMetric[float64]("fragments_chop", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"fragments_flat":        data.NewMetric[float64]("fragments_flat", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"fragments_unsupported": data.NewMetric[float64]("fragments_unsupported", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"hist_opportunities":    data.NewMetric[float64]("hist_opportunities", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"hist_correct_enter":    data.NewMetric[float64]("hist_correct_enter", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"hist_missed_enter":     data.NewMetric[float64]("hist_missed_enter", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"hist_false_enter":      data.NewMetric[float64]("hist_false_enter", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"hist_correct_wait":     data.NewMetric[float64]("hist_correct_wait", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"hist_premature_exit":   data.NewMetric[float64]("hist_premature_exit", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"hist_correct_exit":     data.NewMetric[float64]("hist_correct_exit", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"hist_missed_exit":      data.NewMetric[float64]("hist_missed_exit", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"hist_mean_return":      data.NewMetric[float64]("hist_mean_return", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"hist_return_se":        data.NewMetric[float64]("hist_return_se", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"hist_lower_bound":      data.NewMetric[float64]("hist_lower_bound", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"fwd_enter_predictions": data.NewMetric[float64]("fwd_enter_predictions", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"fwd_correct_enter":     data.NewMetric[float64]("fwd_correct_enter", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"fwd_false_enter":       data.NewMetric[float64]("fwd_false_enter", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"fwd_missed_enter":      data.NewMetric[float64]("fwd_missed_enter", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"fwd_paper_trades":      data.NewMetric[float64]("fwd_paper_trades", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
		"fwd_paper_mean_return": data.NewMetric[float64]("fwd_paper_mean_return", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"fwd_paper_return_se":   data.NewMetric[float64]("fwd_paper_return_se", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"fwd_paper_lower_bound": data.NewMetric[float64]("fwd_paper_lower_bound", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
		"precursor_length":      data.NewMetric[float64]("precursor_length", data.UnitCount, data.TimescaleInstantaneous, 0, 1),
	})

	training.measurement.Label = "learner"
	training.measurement.Metadata["peer-interest"] = "*"
	training.prototype = training.measurement.Clone()
	training.paths = make(map[string]*data.Measurement[float64])

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

	symbol := measurement.Label

	if symbol == "" && measurement.Metadata != nil {
		symbol = measurement.Metadata["symbol"]
	}

	if symbol == "" && training.space.Current != nil {
		symbol = training.space.Current.Symbol
	}

	if symbol == "" {
		return measurement
	}

	if training.paths == nil {
		training.paths = make(map[string]*data.Measurement[float64])
	}

	current, exists := training.paths[symbol]

	if !exists {
		if training.prototype == nil {
			training.Register()
		}

		current = training.prototype.Clone()
		current.Label = symbol
		training.paths[symbol] = current
	}

	if training.detector != nil {
		record, err := training.detector.Process(measurement)

		if err == nil && record != nil {
			training.resolveForwardOutcome(record)
		}
	}

	training.CheckStageGates()

	if reading := training.Rehearsal.published.Load(); reading != nil {
		current.Metrics["decisions"] = current.Metrics["decisions"].Write(float64(reading.Learned))
		current.Metrics["resolved"] = current.Metrics["resolved"].Write(float64(reading.Resolved))
		current.Metrics["unsupported"] = current.Metrics["unsupported"].Write(float64(reading.Unsupported))
		current.Metrics["evaluated"] = current.Metrics["evaluated"].Write(float64(reading.Predicted))

		current.Metrics["fragments_up"] = current.Metrics["fragments_up"].Write(float64(reading.FragmentsUp))
		current.Metrics["fragments_down"] = current.Metrics["fragments_down"].Write(float64(reading.FragmentsDown))
		current.Metrics["fragments_chop"] = current.Metrics["fragments_chop"].Write(float64(reading.FragmentsChop))
		current.Metrics["fragments_flat"] = current.Metrics["fragments_flat"].Write(float64(reading.FragmentsFlat))
		current.Metrics["fragments_unsupported"] = current.Metrics["fragments_unsupported"].Write(float64(reading.FragmentsUnsupported))

		current.Metrics["hist_opportunities"] = current.Metrics["hist_opportunities"].Write(float64(reading.ValidUpOpportunities))
		current.Metrics["hist_correct_enter"] = current.Metrics["hist_correct_enter"].Write(float64(reading.CorrectEnter))
		current.Metrics["hist_missed_enter"] = current.Metrics["hist_missed_enter"].Write(float64(reading.MissedEnter))
		falseEnter := reading.FalseEnterDown + reading.FalseEnterChop + reading.FalseEnterFlat + reading.FalseEnterFriction
		current.Metrics["hist_false_enter"] = current.Metrics["hist_false_enter"].Write(float64(falseEnter))
		correctWait := reading.CorrectWaitDown + reading.CorrectWaitChop + reading.CorrectWaitFlat + reading.CorrectWaitFriction
		current.Metrics["hist_correct_wait"] = current.Metrics["hist_correct_wait"].Write(float64(correctWait))
		current.Metrics["hist_premature_exit"] = current.Metrics["hist_premature_exit"].Write(float64(reading.PrematureExit))
		current.Metrics["hist_correct_exit"] = current.Metrics["hist_correct_exit"].Write(float64(reading.CorrectExit))
		current.Metrics["hist_missed_exit"] = current.Metrics["hist_missed_exit"].Write(float64(reading.MissedExit))

		current.Metrics["hist_mean_return"] = current.Metrics["hist_mean_return"].Write(reading.MeanReturn)
		current.Metrics["hist_return_se"] = current.Metrics["hist_return_se"].Write(reading.ReturnSE)
		current.Metrics["hist_lower_bound"] = current.Metrics["hist_lower_bound"].Write(reading.LowerBound)

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

	current.Metrics["fwd_enter_predictions"] = current.Metrics["fwd_enter_predictions"].Write(float64(training.forwardReading.EnterPredictions))
	current.Metrics["fwd_correct_enter"] = current.Metrics["fwd_correct_enter"].Write(float64(training.forwardReading.CorrectEnter))
	fwdFalseEnter := training.forwardReading.FalseEnterDown + training.forwardReading.FalseEnterChop + training.forwardReading.FalseEnterFlat + training.forwardReading.FalseEnterFriction
	current.Metrics["fwd_false_enter"] = current.Metrics["fwd_false_enter"].Write(float64(fwdFalseEnter))
	current.Metrics["fwd_missed_enter"] = current.Metrics["fwd_missed_enter"].Write(float64(training.forwardReading.MissedEnter))
	current.Metrics["fwd_paper_trades"] = current.Metrics["fwd_paper_trades"].Write(float64(training.forwardReading.PaperTrades))
	current.Metrics["fwd_paper_mean_return"] = current.Metrics["fwd_paper_mean_return"].Write(training.forwardReading.PaperMeanReturn)
	current.Metrics["fwd_paper_return_se"] = current.Metrics["fwd_paper_return_se"].Write(training.forwardReading.PaperReturnSE)
	current.Metrics["fwd_paper_lower_bound"] = current.Metrics["fwd_paper_lower_bound"].Write(training.forwardReading.PaperLowerBound)

	tokens := training.precursor.Tokens(symbol)
	current.Metrics["precursor_length"] = current.Metrics["precursor_length"].Write(float64(len(tokens)))

	stageCode := 0.0

	switch training.stage {
	case StageModelDevelopment:
		stageCode = 0.0
	case StageHistoricalValidation:
		stageCode = 1.0
	case StageForwardPaperLearning:
		stageCode = 2.0
	case StageForwardSkillDemonstrated:
		stageCode = 3.0
	}

	current.Metrics["stage_code"] = current.Metrics["stage_code"].Write(stageCode)
	current.Metadata["stage"] = string(training.stage)
	current.Metadata["stage_blocker"] = training.stageBlocker
	if current.Provenance == nil {
		current.Provenance = make(map[string]string)
	}
	current.Provenance["stage"] = string(training.stage)
	current.Provenance["stage_blocker"] = training.stageBlocker

	current.Metrics["previous_input"] = current.Metrics["previous_input"].Write(float64(training.sequence.Load()))
	current.Metrics["input_count"] = current.Metrics["input_count"].Write(float64(len(measurement.Peers)))
	current.Peers = measurement.Peers
	current.SeqIdx = measurement.SeqIdx
	current.At = measurement.At

	input := func(yield func(unsafe.Pointer) bool) { yield(unsafe.Pointer(measurement)) }

	for output := range training.pipeline.Next(input) {
		evaluation := *(*cognition.Evaluation)(output)
		current.Label = symbol
		current.Result = training.space.Current
		current.SeqIdx = measurement.SeqIdx
		current.At = measurement.At
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
		}

		if action == ActionExit {
			actionValue = 2
		}

		current.Metrics["action"] = current.Metrics["action"].Write(actionValue)

		authorized := false

		if action == ActionEnter {
			minConf := system.UninformativeDirectionConfidence
			plannerConfig := system.NewPlannerConfig()

			if plannerConfig.CognitionSwitchConfidence > minConf {
				minConf = plannerConfig.CognitionSwitchConfidence
			}

			isForwardStage := training.stage == StageForwardPaperLearning || training.stage == StageForwardSkillDemonstrated

			authorized = isForwardStage &&
				evaluation.Support > 0 &&
				evaluation.Confidence > minConf &&
				!evaluation.IsBreak &&
				evaluation.Ambiguity < 0.85
		}

		if action == ActionEnter && !authorized && training.trader != nil {
			reason := fmt.Sprintf("unauthorized in stage %s: %s", training.stage, training.stageBlocker)

			if training.stage == StageForwardPaperLearning || training.stage == StageForwardSkillDemonstrated {
				reason = fmt.Sprintf("authority gate rejected enter: support=%d, conf=%.3f, break=%v, amb=%.3f",
					evaluation.Support, evaluation.Confidence, evaluation.IsBreak, evaluation.Ambiguity)
			}

			training.trader.RecordDecision(
				current.Label,
				"unauthorized",
				evaluation.Confidence,
				reason,
			)
		}

		if training.trader != nil && ((action == ActionEnter && authorized) || action == ActionExit) {
			if action == ActionEnter {
				current.Metrics["agent_entry"] = current.Metrics["agent_entry"].Write(float64(measurement.SeqIdx))
				tokensNow := training.precursor.Tokens(symbol)
				training.freezeForwardPrediction(symbol, evaluation.Context, tokensNow, measurement.SeqIdx, action, evaluation)
			}

			if action == ActionExit {
				current.Metrics["agent_exit"] = current.Metrics["agent_exit"].Write(float64(measurement.SeqIdx))
			}

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

	priceSource := training.price

	if priceSource == nil && training.trader != nil {
		priceSource = training.trader.price
	}

	targetSymbol := symbol

	if priceSource != nil && targetSymbol != "" {
		if mark := priceSource.CurrentMark(targetSymbol); mark != nil {
			current.Metrics["price"] = current.Metrics["price"].Write(mark.Float64())
		}
	}

	if training.Rehearsal != nil {
		if lastRecord := training.Rehearsal.LastExcursion(symbol); lastRecord != nil {
			extType := 0.0

			switch strings.ToUpper(lastRecord.Direction) {
			case "UP":
				extType = 1.0
			case "DOWN":
				extType = 2.0
			case "CHOP":
				extType = 3.0
			case "FLAT":
				extType = 4.0
			}

			current.Metrics["excursion_type"] = current.Metrics["excursion_type"].Write(extType)
			current.Metrics["excursion_mag"] = current.Metrics["excursion_mag"].Write(lastRecord.GrossExcursion * 100)
			current.Metrics["mark_a"] = current.Metrics["mark_a"].Write(float64(lastRecord.PrecursorStartTick))
			current.Metrics["mark_b"] = current.Metrics["mark_b"].Write(float64(lastRecord.AnchorTick))
			current.Metrics["mark_c"] = current.Metrics["mark_c"].Write(float64(lastRecord.ExitTick))
		}
	}

	current.Metrics["invalid_inputs"] = current.Metrics["invalid_inputs"].Write(float64(training.space.Invalid))
	training.sequence.Store(measurement.SeqIdx)
	training.measurement = current
	return current
}

func (training *Training) publishProgress(frame *data.Measurement[float64], record *tables.ExcursionRecord, impulse *grid.Snapshot) {
	if training == nil || frame == nil {
		return
	}

	if training.prototype == nil {
		training.Register()
	}

	if training.prototype == nil {
		return
	}

	current := training.prototype.Clone()
	reading := training.Rehearsal.published.Load()

	if reading != nil {
		current.Metrics["decisions"] = current.Metrics["decisions"].Write(float64(reading.Learned))
		current.Metrics["resolved"] = current.Metrics["resolved"].Write(float64(reading.Resolved))
		current.Metrics["unsupported"] = current.Metrics["unsupported"].Write(float64(reading.Unsupported))
		current.Metrics["evaluated"] = current.Metrics["evaluated"].Write(float64(reading.Predicted))

		current.Metrics["fragments_up"] = current.Metrics["fragments_up"].Write(float64(reading.FragmentsUp))
		current.Metrics["fragments_down"] = current.Metrics["fragments_down"].Write(float64(reading.FragmentsDown))
		current.Metrics["fragments_chop"] = current.Metrics["fragments_chop"].Write(float64(reading.FragmentsChop))
		current.Metrics["fragments_flat"] = current.Metrics["fragments_flat"].Write(float64(reading.FragmentsFlat))
		current.Metrics["fragments_unsupported"] = current.Metrics["fragments_unsupported"].Write(float64(reading.FragmentsUnsupported))

		current.Metrics["hist_opportunities"] = current.Metrics["hist_opportunities"].Write(float64(reading.ValidUpOpportunities))
		current.Metrics["hist_correct_enter"] = current.Metrics["hist_correct_enter"].Write(float64(reading.CorrectEnter))
		current.Metrics["hist_missed_enter"] = current.Metrics["hist_missed_enter"].Write(float64(reading.MissedEnter))
		falseEnter := reading.FalseEnterDown + reading.FalseEnterChop + reading.FalseEnterFlat + reading.FalseEnterFriction
		current.Metrics["hist_false_enter"] = current.Metrics["hist_false_enter"].Write(float64(falseEnter))
		correctWait := reading.CorrectWaitDown + reading.CorrectWaitChop + reading.CorrectWaitFlat + reading.CorrectWaitFriction
		current.Metrics["hist_correct_wait"] = current.Metrics["hist_correct_wait"].Write(float64(correctWait))
		current.Metrics["hist_premature_exit"] = current.Metrics["hist_premature_exit"].Write(float64(reading.PrematureExit))
		current.Metrics["hist_correct_exit"] = current.Metrics["hist_correct_exit"].Write(float64(reading.CorrectExit))
		current.Metrics["hist_missed_exit"] = current.Metrics["hist_missed_exit"].Write(float64(reading.MissedExit))

		current.Metrics["hist_mean_return"] = current.Metrics["hist_mean_return"].Write(reading.MeanReturn)
		current.Metrics["hist_return_se"] = current.Metrics["hist_return_se"].Write(reading.ReturnSE)
		current.Metrics["hist_lower_bound"] = current.Metrics["hist_lower_bound"].Write(reading.LowerBound)

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

	stageCode := 0.0

	switch training.stage {
	case StageModelDevelopment:
		stageCode = 0.0
	case StageHistoricalValidation:
		stageCode = 1.0
	case StageForwardPaperLearning:
		stageCode = 2.0
	case StageForwardSkillDemonstrated:
		stageCode = 3.0
	}

	current.Metrics["stage_code"] = current.Metrics["stage_code"].Write(stageCode)
	current.Metadata["stage"] = string(training.stage)
	current.Metadata["stage_blocker"] = training.stageBlocker
	if current.Provenance == nil {
		current.Provenance = make(map[string]string)
	}
	current.Provenance["stage"] = string(training.stage)
	current.Provenance["stage_blocker"] = training.stageBlocker

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

	if training.focusSymbol != "" && current.Label != training.focusSymbol && current.Label != "learner" {
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

		switch strings.ToUpper(record.Direction) {
		case "UP":
			extType = 1.0
		case "DOWN":
			extType = 2.0
		case "CHOP":
			extType = 3.0
		case "FLAT":
			extType = 4.0
		}

		current.Metrics["excursion_type"] = current.Metrics["excursion_type"].Write(extType)
		current.Metrics["excursion_mag"] = current.Metrics["excursion_mag"].Write(record.GrossExcursion * 100)
		current.Metrics["mark_a"] = current.Metrics["mark_a"].Write(float64(record.PrecursorStartTick))
		current.Metrics["mark_b"] = current.Metrics["mark_b"].Write(float64(record.AnchorTick))
		current.Metrics["mark_c"] = current.Metrics["mark_c"].Write(float64(record.ExitTick))

		actionVal := 0.0

		if record.ClearsFriction {
			actionVal = 1.0
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
