package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/types"
)

type Action = cognition.Action

const (
	ActionEnter = cognition.ActionEnter
	ActionExit  = cognition.ActionExit
	ActionWait  = cognition.ActionWait
)

const TrainingFormat = "symm-training-v1"

// Training stages, progressing as the model matures.
const (
	StageModelDevelopment     = 0
	StageHistoricalValidation = 1
	StageForwardPaper         = 2
	StageSkillDemonstrated    = 3
)

type Training struct {
	*runtime.System
	grid   *store.Grid
	trie   *store.Radix
	engine *cognition.Engine
	trader *Trader
	tee    runtime.Tee
	mutex  sync.RWMutex

	// Stored excursion fragments from the drain's learn callback.
	// Run replays these at hardware speed to train the engine.
	fragments []*data.Measurement[float64]

	// Training state tracking
	steps       atomic.Uint64
	decisions   atomic.Uint64
	resolved    atomic.Uint64
	evaluated   atomic.Uint64
	wins        atomic.Uint64
	totalTrades atomic.Uint64
	stageCode   atomic.Int32

	// Latest evaluation results (written by Step, read by metrics emission)
	lastConfidence atomic.Uint64 // float64 bits
	lastContrast   atomic.Uint64 // float64 bits
	cumReturn      atomic.Uint64 // float64 bits

	// Active position entry tokens mapped by symbol for online refinement.
	activeTokens sync.Map
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	trader *Trader,
	tee runtime.Tee,
) *Training {
	training := &Training{
		System: runtime.NewSystem(ctx, "training", price),
		grid:   store.NewGrid(),
		trie:   store.NewRadix(),
		engine: cognition.NewEngine(cognition.Config{}),
		trader: trader,
		tee:    tee,
	}

	if trader != nil {
		trader.SetOnPositionClosed(func(symbol string, returnFraction float64, fee float64) {
			training.RecordPositionResult(symbol, returnFraction)
		})
	}

	training.Transition(runtime.INIT)
	return training
}


func (training *Training) Register() *data.Measurement[float64] {
	measurement := data.NewMeasurement[float64]("training", nil)
	measurement.Label = types.Focus()
	measurement.Metadata["peer-interest"] = "*"
	return measurement
}

/*
Step develops the grid from live market data and evaluates the trained model.

In the MODEL DEVELOPMENT stage, Step only develops the impulse map grid from
peer signal measurements — it does not train the engine.

Once Run has trained the engine on stored excursion fragments, Step evaluates
the model's predictions. In the FORWARD PAPER stage, Step feeds those predictions
to the trader for paper trading validation.
*/
func (training *Training) Step(
	measurement *data.Measurement[float64],
) *data.Measurement[float64] {
	if measurement == nil {
		return nil
	}

	// Ensure the measurement has a timestamp for the dashboard.
	if measurement.At.IsZero() {
		measurement.At = time.Now().UTC()
	}

	training.steps.Add(1)

	peers := measurement.Peers

	if len(peers) == 0 && len(measurement.Metrics) > 0 {
		peers = []*data.Measurement[float64]{measurement}
	}

	for _, peer := range peers {
		if peer == nil || peer.Metrics == nil || len(peer.Metrics) == 0 {
			continue
		}

		if measurement.Label == "" {
			if peer.Label != "" {
				measurement.Label = peer.Label
			}

			if peer.Label == "" {
				measurement.Label = types.Focus()
			}
		}

		// Update grid and stamp lattice coordinates (X, Y) and Region on peer metrics.
		training.grid.Update(peer)
	}

	if len(measurement.Metrics) > 0 {
		training.grid.Update(measurement)
	}

	// Forward price, spread, and excursion metadata from peers matching this measurement's symbol.
	targetSymbol := measurement.Label

	if targetSymbol == "" {
		targetSymbol = types.Focus()
	}

	for _, peer := range peers {
		if peer == nil || peer == measurement {
			continue
		}

		if peer.Label != targetSymbol {
			continue
		}

		if _, already := measurement.Metrics["price"]; !already {
			for _, key := range []string{"price", "last_price", "last", "spot_price", "reference_price", "midpoint"} {
				if priceMetric, found := peer.Metrics[key]; found && priceMetric.Raw > 0 {
					priceMetric.Label = "price"
					measurement.Metrics["price"] = priceMetric
					break
				}
			}
		}

		if _, already := measurement.Metrics["spread"]; !already {
			for _, key := range []string{"spread", "relative_spread"} {
				if spreadMetric, found := peer.Metrics[key]; found && spreadMetric.Raw > 0 {
					spreadMetric.Label = "spread"
					measurement.Metrics["spread"] = spreadMetric
					break
				}
			}
		}

		if peer.Metadata != nil && peer.Metadata["excursion"] != "" {
			if measurement.Metadata == nil {
				measurement.Metadata = make(map[string]string)
			}

			measurement.Metadata["excursion"] = peer.Metadata["excursion"]
			measurement.Metadata["excursion_start"] = peer.Metadata["excursion_start"]
			measurement.Metadata["excursion_ignition"] = peer.Metadata["excursion_ignition"]
			measurement.Metadata["excursion_end"] = peer.Metadata["excursion_end"]
		}
	}

	// In StageModelDevelopment, Step only develops the grid until it settles.
	// Training does NOT evaluate predictions or emit actions until the grid has settled.
	if training.stageCode.Load() == StageModelDevelopment {
		if training.grid.Settled {
			if err := training.SaveCheckpoint(); err != nil {
				errnie.Error(err)
			}

			training.stageCode.Store(StageHistoricalValidation)
		}

		if !training.grid.Settled {
			if measurement.Provenance == nil {
				measurement.Provenance = make(map[string]string)
			}

			measurement.Provenance["stage"] = training.stageName()
			measurement.Provenance["stage_blocker"] = training.stageBlocker()
			training.emitStateMetrics(measurement)
			return measurement
		}
	}

	var topRegion uint8
	var maxActivity float64
	var regionActivity [256]float64

	for _, peer := range peers {
		if peer == nil {
			continue
		}

		for label, metric := range peer.Metrics {
			region := training.grid.Region(label)

			if region > 0 {
				activity := math.Abs(metric.Raw)
				regionActivity[region] += activity

				if regionActivity[region] > maxActivity {
					maxActivity = regionActivity[region]
					topRegion = region
				}
			}
		}
	}

	token := training.grid.LitRegions(measurement, 3)
	if len(token) == 0 && topRegion > 0 {
		token = []byte{topRegion}
	}
	if len(token) == 0 {
		reg := training.grid.Region("price")
		if reg == 0 {
			reg = 1
		}
		token = []byte{reg}
	}

	var evalConfidence, evalContrast float64

	if len(token) > 0 {
		action := ActionWait

		evalResult, evalErr := training.engine.Evaluate(token)

		if evalErr == nil && evalResult.Evaluation.WinnerClass != "" {
			action = Action(evalResult.Evaluation.WinnerClass)
			evalConfidence = evalResult.Evaluation.Confidence
			evalContrast = evalResult.Evaluation.Contrast
			training.evaluated.Add(1)
		}

		if action == ActionWait || action == "" {
			actionBytes, found := training.trie.Get(token)

			if found && len(actionBytes) > 0 {
				action = Action(actionBytes)
			}
		}

		if action == ActionEnter {
			training.activeTokens.Store(measurement.Label, token)
			training.activeTokens.Store("__latest__", token)
		}

		// Only act on predictions in the paper trading stage.
		if action != ActionWait && training.trader != nil && training.stageCode.Load() >= StageForwardPaper {
			training.trader.OnAction(measurement.Label, action)
		}

		if evalErr == nil && evalResult.Evaluation.WinnerClass != "" {
			measurement.Provenance["confidence"] = fmt.Sprintf("%.2f", evalResult.Evaluation.Confidence)
			measurement.Provenance["surprisal"] = fmt.Sprintf("%.2f", evalResult.Evaluation.Surprisal)
		}

		var actionVal float64

		if action == ActionEnter {
			actionVal = 1
		}

		if action == ActionExit {
			actionVal = 2
		}

		actionMetric := data.NewMetric[float64](
			"action", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		)

		actionMetric.Raw = actionVal
		measurement.Metrics["action"] = actionMetric

		// Store latest evaluation values atomically for metrics emission.
		training.lastConfidence.Store(math.Float64bits(evalConfidence))
		training.lastContrast.Store(math.Float64bits(evalContrast))
	}

	// Always set provenance so the dashboard receives stage info
	// regardless of whether the grid has active regions.
	if measurement.Provenance == nil {
		measurement.Provenance = make(map[string]string)
	}

	measurement.Provenance["stage"] = training.stageName()
	measurement.Provenance["stage_blocker"] = training.stageBlocker()

	// Emit training state metrics the dashboard reads.
	training.emitStateMetrics(measurement)

	// Advance the stage machine.
	training.advanceStage()

	// Peers transport the grid structure to the frontend. Do not clear them.
	return measurement
}


/*
Run replays stored excursion fragments at hardware speed to train the model.
*/
func (training *Training) Run() {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-training.Context().Done():
				return
			case <-ticker.C:
				training.trainFromFragments()
			}
		}
	}()
}

func (training *Training) trainFromFragments() {
	if !training.grid.Settled {
		return
	}

	training.mutex.RLock()

	if len(training.fragments) == 0 {
		training.mutex.RUnlock()
		return
	}

	fragments := make([]*data.Measurement[float64], len(training.fragments))
	copy(fragments, training.fragments)
	training.mutex.RUnlock()

	type excursionGroup struct {
		direction      string
		clearsFriction bool
		category       string
		startTick      int64
		ignitionTick   int64
		endTick        int64
		measurements   []*data.Measurement[float64]
	}

	groups := make(map[string]*excursionGroup)

	for _, fragment := range fragments {
		if fragment == nil || fragment.Metadata == nil {
			continue
		}

		direction := fragment.Metadata["excursion"]

		if direction == "" {
			continue
		}

		ignitionStr := fragment.Metadata["excursion_ignition"]

		if ignitionStr == "" {
			ignitionStr = fragment.Label
		}

		key := fmt.Sprintf("%s-%s", fragment.Label, ignitionStr)
		group, found := groups[key]

		if !found {
			startTick, _ := strconv.ParseInt(fragment.Metadata["excursion_start"], 10, 64)
			ignitionTick, _ := strconv.ParseInt(fragment.Metadata["excursion_ignition"], 10, 64)
			endTick, _ := strconv.ParseInt(fragment.Metadata["excursion_end"], 10, 64)

			group = &excursionGroup{
				direction:      direction,
				clearsFriction: fragment.Metadata["excursion_clears_friction"] == "true",
				category:       fragment.Metadata["excursion_category"],
				startTick:      startTick,
				ignitionTick:   ignitionTick,
				endTick:        endTick,
			}
			groups[key] = group
		}

		group.measurements = append(group.measurements, fragment)
	}

	trained := false

	// Replay excursion fragments across multiple passes at hardware speed.
	// In each pass, randomize the A marker somewhat (as long as A < B),
	// training a robust, scale-invariant model across varying precursor horizons.
	const passes = 3

	for _, group := range groups {
		if group == nil || len(group.measurements) == 0 {
			continue
		}

		var targetClass []byte

		if group.direction == "upper" {
			if group.clearsFriction || group.category == "upper_profitable" {
				targetClass = []byte(ActionEnter)
			}

			if !group.clearsFriction && group.category != "upper_profitable" {
				targetClass = []byte(ActionWait)
			}
		}

		if group.direction == "lower" {
			targetClass = []byte(ActionExit)
		}

		if group.direction == "chop" || group.direction == "flat" {
			targetClass = []byte(ActionWait)
		}

		if len(targetClass) == 0 {
			targetClass = []byte(ActionWait)
		}

		for pass := 0; pass < passes; pass++ {
			// Elastic A-marker jitter:
			// Pass 0 uses the exact initial precursor start.
			// Subsequent passes jitter the effective A marker forward within [A, B-1].
			var activeMeas []*data.Measurement[float64]

			if len(group.measurements) > 1 && group.ignitionTick > group.startTick+1 {
				span := group.ignitionTick - group.startTick
				jitterOffset := int64(0)

				if pass > 0 {
					fraction := float64(pass) / float64(passes+1)
					jitterOffset = int64(float64(span) * fraction)
				}

				effectiveA := group.startTick + jitterOffset

				if effectiveA >= group.ignitionTick {
					effectiveA = group.ignitionTick - 1
				}

				for _, meas := range group.measurements {
					if meas == nil {
						continue
					}

					tick := meas.SeqIdx

					if tick == 0 && meas.Metadata != nil {
						if ts, err := strconv.ParseInt(meas.Metadata["tick"], 10, 64); err == nil {
							tick = ts
						}
					}

					// Include precursor observations from the effective A marker up to ignition B
					if tick >= effectiveA && (group.ignitionTick == 0 || tick <= group.ignitionTick) {
						activeMeas = append(activeMeas, meas)
					}
				}
			}

			if len(activeMeas) == 0 {
				activeMeas = group.measurements
			}

			// Extract trajectory sequence of lit regions across the precursor window
			var token []byte

			for _, meas := range activeMeas {
				lit := training.grid.LitRegions(meas, 1)

				if len(lit) > 0 {
					if len(token) == 0 || token[len(token)-1] != lit[0] {
						token = append(token, lit[0])
					}
				}
			}

			if len(token) == 0 {
				token = training.grid.LitRegions(activeMeas[len(activeMeas)-1], 3)
			}

			if len(token) == 0 {
				reg := training.grid.Region("price")

				if reg == 0 {
					reg = 1
				}

				token = []byte{byte(reg)}
			}

			if _, err := training.engine.Train(token, targetClass, 1.0); err != nil {
				errnie.Error(err)
				continue
			}

			training.trie.Insert(token, targetClass)
			training.decisions.Add(1)
			trained = true
		}
	}

	if trained {
		if _, _, _, _, err := training.engine.Consolidate(1.0); err != nil {
			errnie.Error(err)
		}

		training.engine.Prune(0.05)
		training.advanceStage()

		if err := training.SaveCheckpoint(); err != nil {
			errnie.Error(err)
		}
	}
}


/*
stageName returns the human-readable name of the current training stage.
*/
func (training *Training) stageName() string {
	switch training.stageCode.Load() {
	case StageHistoricalValidation:
		return "HISTORICAL VALIDATION"
	case StageForwardPaper:
		return "FORWARD PAPER LEARNING"
	case StageSkillDemonstrated:
		return "FORWARD SKILL DEMONSTRATED"
	default:
		return "MODEL DEVELOPMENT"
	}
}

/*
stageBlocker returns a description of what the current stage is waiting for.
*/
func (training *Training) stageBlocker() string {
	switch training.stageCode.Load() {
	case StageModelDevelopment:
		if !training.grid.Settled {
			return "waiting for grid regions to settle"
		}

		if training.decisions.Load() == 0 {
			return "collecting initial excursion fragments"
		}

		return "accumulating excursion training data"
	case StageHistoricalValidation:
		return "replaying excursion fragments for validation"
	case StageForwardPaper:
		return "paper trading into real-time market"
	case StageSkillDemonstrated:
		return "edge demonstrated"
	default:
		return "collecting initial historical development samples"
	}
}

/*
advanceStage checks whether the training stage should be promoted based on
measured model state. Stage transitions are one-way.
*/
func (training *Training) advanceStage() {
	current := training.stageCode.Load()

	if current >= StageSkillDemonstrated {
		return
	}

	switch current {
	case StageModelDevelopment:
		// Advance when the grid has settled and the engine has learned
		// at least one association.
		if training.grid.Settled && training.engine.Len() > 0 {
			training.stageCode.CompareAndSwap(StageModelDevelopment, StageHistoricalValidation)
		}
	case StageHistoricalValidation:
		// Advance to paper trading when the engine has accumulated
		// sufficient distinct associations (≥10 trie entries).
		if training.engine.Len() >= 10 {
			training.stageCode.CompareAndSwap(StageHistoricalValidation, StageForwardPaper)
		}
	case StageForwardPaper:
		// Advance when the model has demonstrated positive edge:
		// wins > 50% of total trades and at least 5 trades completed.
		total := training.totalTrades.Load()
		wins := training.wins.Load()

		if total >= 5 && wins*2 > total {
			training.stageCode.CompareAndSwap(StageForwardPaper, StageSkillDemonstrated)
		}
	}
}

/*
emitStateMetrics writes the training state counters and evaluation results
into the outgoing measurement so the dashboard reads real data.
*/
func (training *Training) emitStateMetrics(measurement *data.Measurement[float64]) {
	if measurement.Metrics == nil {
		measurement.Metrics = make(map[string]data.Metric[float64])
	}

	if measurement.Provenance == nil {
		measurement.Provenance = make(map[string]string)
	}

	measurement.Provenance["stage_blocker"] = training.stageBlocker()

	stageMetric := data.NewMetric[float64](
		"stage_code", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
	)
	stageMetric.Raw = float64(training.stageCode.Load())
	measurement.Metrics["stage_code"] = stageMetric

	stepsMetric := data.NewMetric[float64](
		"steps", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
	)
	stepsMetric.Raw = float64(training.steps.Load())
	measurement.Metrics["steps"] = stepsMetric

	decisionsMetric := data.NewMetric[float64](
		"decisions", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
	)
	decisionsMetric.Raw = float64(training.decisions.Load())
	measurement.Metrics["decisions"] = decisionsMetric

	resolvedMetric := data.NewMetric[float64](
		"resolved", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
	)
	resolvedMetric.Raw = float64(training.resolved.Load())
	measurement.Metrics["resolved"] = resolvedMetric

	evaluatedMetric := data.NewMetric[float64](
		"evaluated", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
	)
	evaluatedMetric.Raw = float64(training.evaluated.Load())
	measurement.Metrics["evaluated"] = evaluatedMetric

	confidence := math.Float64frombits(training.lastConfidence.Load())
	confidenceMetric := data.NewMetric[float64](
		"confidence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
	)
	confidenceMetric.Raw = confidence
	measurement.Metrics["confidence"] = confidenceMetric

	contrast := math.Float64frombits(training.lastContrast.Load())
	contrastMetric := data.NewMetric[float64](
		"contrast", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
	)
	contrastMetric.Raw = contrast
	measurement.Metrics["contrast"] = contrastMetric

	// Edge is the real realized average return fraction across closed trades, 0 if no trades.
	totalTrades := training.totalTrades.Load()
	var edge float64

	if totalTrades > 0 {
		edge = math.Float64frombits(training.cumReturn.Load()) / float64(totalTrades)
	}

	edgeMetric := data.NewMetric[float64](
		"edge", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
	)
	edgeMetric.Raw = edge
	measurement.Metrics["edge"] = edgeMetric

	// Win rate = wins / total trades, 0 if no trades.
	var winRate float64

	if totalTrades > 0 {
		winRate = float64(training.wins.Load()) / float64(totalTrades)
	}

	winRateMetric := data.NewMetric[float64](
		"win_rate", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
	)
	winRateMetric.Raw = winRate
	measurement.Metrics["win_rate"] = winRateMetric

	// Trading active flag.
	var tradingVal float64

	if training.stageCode.Load() >= StageForwardPaper && training.trader != nil {
		tradingVal = 1
	}

	tradingMetric := data.NewMetric[float64](
		"trading", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
	)
	tradingMetric.Raw = tradingVal
	measurement.Metrics["trading"] = tradingMetric

	// Excursion markers for tape visualization
	if measurement.Metadata != nil {
		if exc := measurement.Metadata["excursion"]; exc != "" {
			var extType float64
			if exc == "upper" {
				extType = 1
			}
			if exc == "lower" {
				extType = 2
			}
			extMetric := data.NewMetric[float64](
				"excursion_type", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
			)
			extMetric.Raw = extType
			measurement.Metrics["excursion_type"] = extMetric

			if startStr := measurement.Metadata["excursion_start"]; startStr != "" {
				if st, err := strconv.ParseFloat(startStr, 64); err == nil {
					ma := data.NewMetric[float64]("mark_a", data.UnitCount, data.TimescaleInstantaneous, 0, 1)
					ma.Raw = st
					measurement.Metrics["mark_a"] = ma
				}
			}

			if ignStr := measurement.Metadata["excursion_ignition"]; ignStr != "" {
				if ig, err := strconv.ParseFloat(ignStr, 64); err == nil {
					mb := data.NewMetric[float64]("mark_b", data.UnitCount, data.TimescaleInstantaneous, 0, 1)
					mb.Raw = ig
					measurement.Metrics["mark_b"] = mb
				}
			}

			if endStr := measurement.Metadata["excursion_end"]; endStr != "" {
				if en, err := strconv.ParseFloat(endStr, 64); err == nil {
					mc := data.NewMetric[float64]("mark_c", data.UnitCount, data.TimescaleInstantaneous, 0, 1)
					mc.Raw = en
					measurement.Metrics["mark_c"] = mc
				}
			}
		}
	}
}

/*
RecordPositionResult processes a closed position outcome from the broker desk.
It updates win/loss counters and uses the realized PnL to refine the trie model
for the precursor token that initiated the entry.
*/
func (training *Training) RecordPositionResult(symbol string, returnFraction float64) {
	training.totalTrades.Add(1)
	training.resolved.Add(1)

	for {
		oldBits := training.cumReturn.Load()
		newVal := math.Float64frombits(oldBits) + returnFraction

		if training.cumReturn.CompareAndSwap(oldBits, math.Float64bits(newVal)) {
			break
		}
	}

	if returnFraction > 0 {
		training.wins.Add(1)
	}

	// Retrieve the precursor token that triggered this entry.
	var token []byte

	if symbol != "" {
		if rawToken, found := training.activeTokens.LoadAndDelete(symbol); found {
			if tokenBytes, ok := rawToken.([]byte); ok {
				token = tokenBytes
			}
		}
	}

	if len(token) == 0 {
		if rawToken, found := training.activeTokens.LoadAndDelete("__latest__"); found {
			if tokenBytes, ok := rawToken.([]byte); ok {
				token = tokenBytes
			}
		}
	}

	if len(token) > 0 {
		targetClass := []byte(ActionWait)
		weight := math.Abs(returnFraction)

		if weight <= 0 {
			weight = 0.1
		}

		if returnFraction > 0 {
			targetClass = []byte(ActionEnter)
			weight = 1.0 + returnFraction
		}

		if _, err := training.engine.Train(token, targetClass, weight); err != nil {
			errnie.Error(err)
		}

		training.trie.Insert(token, targetClass)

		if _, _, _, _, err := training.engine.Consolidate(1.0); err != nil {
			errnie.Error(err)
		}

		training.engine.Prune(0.05)
		training.advanceStage()

		if err := training.SaveCheckpoint(); err != nil {
			errnie.Error(err)
		}
	}
}

/*
RecordTradeResult is the callback used to report closed position outcomes.
It delegates to RecordPositionResult with an empty symbol.
*/
func (training *Training) RecordTradeResult(returnFraction float64) {
	training.RecordPositionResult("", returnFraction)
}

/*
ExcursionLearn is the learn callback for catalog.Drain. It receives
StoreTee-tagged measurements carrying excursion metadata and stores them
as fragments for Run to replay at hardware speed. Returns excursion records
for persistence in the Iceberg catalog only when an excursion completes.
*/
func (training *Training) ExcursionLearn(
	measurement *data.Measurement[float64],
) ([]tables.ExcursionRecord, error) {
	if measurement == nil {
		return nil, nil
	}

	excursionDirection := measurement.Metadata["excursion"]

	if excursionDirection == "" {
		return nil, nil
	}

	// Store the tagged measurement as a fragment for Run to replay.
	training.mutex.Lock()
	training.fragments = append(training.fragments, measurement.Clone())
	training.mutex.Unlock()

	// Only return an ExcursionRecord when the excursion has completed!
	if measurement.Metadata["excursion_event"] != "completed" {
		return nil, nil
	}

	var startTick, ignitionTick, extremumTick, endTick int64

	if metaVal := measurement.Metadata["excursion_start"]; metaVal != "" {
		startTick, _ = strconv.ParseInt(metaVal, 10, 64)
	}

	if metaVal := measurement.Metadata["excursion_ignition"]; metaVal != "" {
		ignitionTick, _ = strconv.ParseInt(metaVal, 10, 64)
	}

	if metaVal := measurement.Metadata["excursion_extremum_tick"]; metaVal != "" {
		extremumTick, _ = strconv.ParseInt(metaVal, 10, 64)
	}

	if metaVal := measurement.Metadata["excursion_end"]; metaVal != "" {
		endTick, _ = strconv.ParseInt(metaVal, 10, 64)
	}

	var priceVal float64

	for _, key := range []string{"price", "last_price", "last", "spot_price", "reference_price", "midpoint"} {
		if metric, ok := measurement.Metrics[key]; ok && metric.Raw > 0 {
			priceVal = metric.Raw
			break
		}
	}

	var entryPrice, extremumPrice, exitPrice float64

	if metaVal := measurement.Metadata["excursion_entry_price"]; metaVal != "" {
		entryPrice, _ = strconv.ParseFloat(metaVal, 64)
	}

	if metaVal := measurement.Metadata["excursion_extremum_price"]; metaVal != "" {
		extremumPrice, _ = strconv.ParseFloat(metaVal, 64)
	}

	if metaVal := measurement.Metadata["excursion_exit_price"]; metaVal != "" {
		exitPrice, _ = strconv.ParseFloat(metaVal, 64)
	}

	if entryPrice <= 0 {
		entryPrice = priceVal
	}

	if extremumPrice <= 0 {
		extremumPrice = priceVal
	}

	if exitPrice <= 0 {
		exitPrice = priceVal
	}

	if extremumTick <= 0 {
		extremumTick = ignitionTick
	}

	positionSize := 40.0

	if metaVal := measurement.Metadata["excursion_position_size"]; metaVal != "" {
		if posSize, err := strconv.ParseFloat(metaVal, 64); err == nil && posSize > 0 {
			positionSize = posSize
		}
	}

	var fee, profit, profitFraction, grossExcursion float64

	if metaVal := measurement.Metadata["excursion_fee"]; metaVal != "" {
		fee, _ = strconv.ParseFloat(metaVal, 64)
	}

	if metaVal := measurement.Metadata["excursion_profit"]; metaVal != "" {
		profit, _ = strconv.ParseFloat(metaVal, 64)
	}

	if metaVal := measurement.Metadata["excursion_profit_fraction"]; metaVal != "" {
		profitFraction, _ = strconv.ParseFloat(metaVal, 64)
	}

	if metaVal := measurement.Metadata["excursion_gross"]; metaVal != "" {
		grossExcursion, _ = strconv.ParseFloat(metaVal, 64)
	}

	clearsFriction := measurement.Metadata["excursion_clears_friction"] == "true"

	if profit > 0 && (excursionDirection == "upper" || measurement.Metadata["excursion_category"] == "upper_profitable") {
		clearsFriction = true
	}

	direction := excursionDirection

	if direction == "upper" {
		direction = "upward"
	}
	if direction == "lower" {
		direction = "downward"
	}

	var record tables.ExcursionRecord
	record.ID = fmt.Sprintf("exc-%s-%d", measurement.Label, ignitionTick)
	record.Symbol = measurement.Label
	record.Direction = direction
	record.PrecursorStartTick = startTick
	record.AnchorTick = ignitionTick
	record.ExtremumTick = extremumTick
	record.ExitTick = endTick
	record.PostEndTick = measurement.SeqIdx
	record.EntryPrice = entryPrice
	record.ExtremumPrice = extremumPrice
	record.ExitPrice = exitPrice
	record.PositionSize = positionSize
	record.GrossExcursion = grossExcursion
	record.Fee = fee
	record.Profit = profit
	record.ProfitFraction = profitFraction
	record.ClearsFriction = clearsFriction
	record.ObservationCount = measurement.SeqIdx - startTick + 1
	record.Status = "completed"

	training.trainOnExcursion(record)

	return []tables.ExcursionRecord{record}, nil
}

func (training *Training) trainOnExcursion(record tables.ExcursionRecord) {
	if !training.grid.Settled {
		return
	}

	var token []byte

	training.mutex.RLock()
	for idx := len(training.fragments) - 1; idx >= 0; idx-- {
		frag := training.fragments[idx]
		if frag != nil && frag.Label == record.Symbol {
			lit := training.grid.LitRegions(frag, 3)
			if len(lit) > 0 {
				token = lit
				break
			}
		}
	}
	training.mutex.RUnlock()

	if len(token) == 0 {
		region := training.grid.Region("price")
		if region == 0 {
			region = 1
		}
		token = []byte{byte(region)}
	}

	var targetClass []byte

	switch record.Direction {
	case "upward", "upper":
		if record.ClearsFriction {
			targetClass = []byte(ActionEnter)
		}

		if !record.ClearsFriction {
			targetClass = []byte(ActionWait)
		}
	case "downward", "lower":
		targetClass = []byte(ActionExit)
	case "chop", "flat":
		targetClass = []byte(ActionWait)
	default:
		targetClass = []byte(ActionWait)
	}

	if _, err := training.engine.Train(token, targetClass, 1.0); err != nil {
		errnie.Error(err)
		return
	}

	training.trie.Insert(token, targetClass)
	training.decisions.Add(1)

	if _, _, _, _, err := training.engine.Consolidate(1.0); err != nil {
		errnie.Error(err)
	}

	training.engine.Prune(0.05)
	training.advanceStage()

	if err := training.SaveCheckpoint(); err != nil {
		errnie.Error(err)
	}
}

var checkpointMu sync.Mutex

func (training *Training) SaveCheckpoint() error {
	checkpointMu.Lock()
	defer checkpointMu.Unlock()

	var modelBytes []byte

	if training.engine != nil {
		if snap, err := training.engine.Snapshot(); err == nil {
			modelBytes = snap.Model
		}
	}

	payload, err := json.MarshalIndent(struct {
		Grid        *store.Grid  `json:"grid"`
		Trie        *store.Radix `json:"trie"`
		EngineModel []byte       `json:"engine_model,omitempty"`
	}{
		Grid:        training.grid,
		Trie:        training.trie,
		EngineModel: modelBytes,
	}, "", "  ")

	if err != nil {
		return errnie.Error(err)
	}

	return os.WriteFile("grid_checkpoint.json", payload, 0644)
}

func (training *Training) LoadCheckpoint() error {
	checkpointMu.Lock()
	defer checkpointMu.Unlock()

	payload, err := os.ReadFile("grid_checkpoint.json")

	if err != nil {
		return err
	}

	state := struct {
		Grid        *store.Grid  `json:"grid"`
		Trie        *store.Radix `json:"trie"`
		EngineModel []byte       `json:"engine_model,omitempty"`
	}{
		Grid: training.grid,
		Trie: training.trie,
	}

	if err := json.Unmarshal(payload, &state); err != nil {
		return errnie.Error(err)
	}

	if len(state.EngineModel) > 0 && training.engine != nil {
		if _, err := training.engine.Restore(state.EngineModel); err != nil {
			errnie.Error(err)
		}
	}

	if training.grid.Settled {
		training.Transition(runtime.READY)
	}

	return nil
}

func (training *Training) CognitionTree() cognition.CognitionTreeExport {
	training.mutex.RLock()
	defer training.mutex.RUnlock()

	if training.engine != nil {
		tree := training.engine.TreeExport()

		if len(tree.Branches) > 0 || len(tree.Feasible) > 0 {
			return tree
		}
	}

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
