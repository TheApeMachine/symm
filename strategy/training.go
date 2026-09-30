package strategy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/types"
)

type Action = string

const TrainingFormat = "symm-training-v1"

const (
	ActionEnter Action = "enter"
	ActionExit  Action = "exit"
	ActionWait  Action = "wait"
)

const (
	StageGridSettling       int32 = 0
	StageHistoricalTraining int32 = 1
	StageForwardPaper       int32 = 2
	StageSkillDemonstrated  int32 = 3
)

// Keep backward compat aliases for the old constant names used by tests / UI.
const (
	StageModelDevelopment     = StageGridSettling
	StageHistoricalValidation = StageHistoricalTraining
)

/*
Training is the learning orchestrator. Its lifecycle follows three stages:

  - Stage 0 (GRID_SETTLING): Step() feeds every arriving measurement into
    the grid, building regions. Nothing else happens.
  - Stage 1 (HISTORICAL_TRAINING): The grid is frozen. Run() starts a
    background goroutine that reads excursion fragments from the catalog and
    trains the radix trie on ground-truth labelled enter/exit/wait actions.
  - Stage 2 (FORWARD_PAPER): Step() uses the frozen grid + trained trie to
    make real-time predictions. Paper trading validates the model.
*/
type Training struct {
	*runtime.System
	grid   *store.Grid
	trie   *store.Radix
	trader *Trader
	tee    runtime.Tee
	mutex  sync.RWMutex
	priors [][]byte

	// Fragments tracked during a live or drained excursion
	fragments []*data.Measurement[float64]

	stageCode   atomic.Int32
	steps       atomic.Uint64
	decisions   atomic.Uint64
	evaluated   atomic.Uint64
	totalTrades atomic.Uint64
	wins        atomic.Uint64
	iterations  atomic.Uint64

	// Catalog reference and epoch for historical replay.
	catalog *tables.Catalog
	epoch   int64

	learnQueue chan tables.ExcursionRecord
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	trader *Trader,
	tee runtime.Tee,
) *Training {
	training := &Training{
		System:     runtime.NewSystem(ctx, "training", price),
		grid:       store.NewGrid(),
		trie:       store.NewRadix(),
		trader:     trader,
		tee:        tee,
		priors:     make([][]byte, 0, 3),
		fragments:  make([]*data.Measurement[float64], 0),
		learnQueue: make(chan tables.ExcursionRecord, 100000),
	}

	if trader != nil {
		trader.SetOnPositionClosed(func(symbol string, returnFraction float64, fee float64) {
			training.RecordPositionResult(symbol, returnFraction)
		})
	}

	training.Transition(runtime.INIT)
	return training
}

// SetCatalog wires the catalog and epoch for historical training replay.
func (training *Training) SetCatalog(catalog *tables.Catalog, epoch int64) {
	training.catalog = catalog
	training.epoch = epoch
}

func (training *Training) Register() *data.Measurement[float64] {
	measurement := data.NewMeasurement[float64]("training", nil)
	measurement.Label = types.Focus()
	measurement.Metadata["peer-interest"] = "*"
	return measurement
}

/*
Run starts the background training goroutines. It processes two streams:

  - The learn queue: excursion records detected in real-time by the StoreTee
    and fed through ExcursionLearn.
  - Historical replay: once the grid settles, reads stored excursion fragments
    from the catalog and trains on them at hardware speed.
*/
func (training *Training) Run() {
	// Live excursion learn queue consumer
	go func() {
		for {
			select {
			case <-training.Context().Done():
				return
			case record := <-training.learnQueue:
				training.trainOnExcursion(record)
			}
		}
	}()
}

/*
Step is part of the LMAX Disruptor pipeline. Its behavior depends on the
current stage:

  - Stage 0: Feed measurements into the grid for settling. Once settled,
    save checkpoint and transition to Stage 1, kicking off historical training.
  - Stage 1: Emit wait actions while historical training runs in background.
    Still feed region tokens to build context but don't act on them yet.
  - Stage 2+: Use the frozen grid + trained trie to predict actions on the
    live market tape. Forward predictions to the trader for paper trading.
*/
func (training *Training) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if measurement == nil {
		return nil
	}

	training.steps.Add(1)

	// Feed all peer measurements into the grid
	peers := measurement.Peers
	for _, peer := range peers {
		if peer != nil && len(peer.Metrics) > 0 {
			training.grid.Update(peer)
		}
	}

	if len(measurement.Metrics) > 0 {
		training.grid.Update(measurement)
	}

	// Stage 0: Grid settling — nothing else
	if training.stageCode.Load() == StageGridSettling {
		if training.grid.Settled {
			if err := training.SaveCheckpoint(); err != nil {
				errnie.Error(err)
			}
			training.stageCode.Store(StageHistoricalTraining)
			errnie.Info("training: grid settled, starting historical training")

			// Kick off historical training in background
			go training.runHistoricalTraining()
		}

		if !training.grid.Settled {
			training.emitMetrics(measurement, ActionWait)
			return measurement
		}
	}

	// Generate region token from the frozen grid
	token := training.grid.LitRegions(measurement, 3)
	if len(token) == 0 {
		training.emitMetrics(measurement, ActionWait)
		return measurement
	}

	// Build temporal signature from priors
	training.mutex.Lock()
	if len(training.priors) > 0 && bytes.Equal(training.priors[len(training.priors)-1], token) {
		// Do not append duplicate consecutive states
	} else {
		if len(training.priors) >= 3 {
			training.priors = training.priors[1:]
		}
		training.priors = append(training.priors, token)
	}

	var sig []byte
	for _, p := range training.priors {
		sig = append(sig, p...)
		sig = append(sig, 0x00) // temporal delimiter
	}
	sig = append(sig, token...)
	training.mutex.Unlock()

	action := ActionWait

	if trieVal, ok := training.trie.Get(sig); ok && len(trieVal) > 0 {
		action = string(trieVal)
		training.evaluated.Add(1)
	}

	symbol := measurement.Label
	if symbol == "" {
		symbol = types.Focus()
	}

	// Stage 2+: Forward paper trading
	if action != ActionWait && training.trader != nil && training.stageCode.Load() >= StageForwardPaper {
		training.trader.OnAction(symbol, action)
	}

	training.emitMetrics(measurement, action)
	return measurement
}

/*
runHistoricalTraining reads excursion records from the catalog and trains
on them at hardware speed. Each excursion is replayed multiple times with
random A-offsets for robustness. Once sufficient skill is demonstrated,
transitions to Stage 2 (forward paper).
*/
func (training *Training) runHistoricalTraining() {
	if training.catalog == nil {
		errnie.Info("training: no catalog available, skipping historical training")
		training.stageCode.Store(StageForwardPaper)
		return
	}

	ctx := training.Context()
	const iterationsPerExcursion = 5
	const minIterationsForForward = 50

	for iteration := 0; ctx.Err() == nil; iteration++ {
		excursions, err := training.catalog.Excursions(ctx, training.epoch, nil)
		if err != nil {
			errnie.Error(err)
			break
		}

		if len(excursions) == 0 {
			errnie.Info("training: no excursion records found, will retry")
			// Wait for some excursions to be detected and stored
			select {
			case <-ctx.Done():
				return
			case record := <-training.learnQueue:
				training.trainOnExcursion(record)
				continue
			}
		}

		for _, excursion := range excursions {
			if ctx.Err() != nil {
				return
			}

			for rep := 0; rep < iterationsPerExcursion; rep++ {
				training.replayExcursion(ctx, excursion, rep)
				training.iterations.Add(1)
			}
		}

		// Checkpoint after each full pass
		if err := training.SaveCheckpoint(); err != nil {
			errnie.Error(err)
		}

		totalIterations := training.iterations.Load()

		if totalIterations >= uint64(minIterationsForForward) && training.stageCode.Load() < StageForwardPaper {
			training.stageCode.Store(StageForwardPaper)
			errnie.Info(fmt.Sprintf(
				"training: %d iterations completed, advancing to forward paper",
				totalIterations,
			))
		}
	}
}

/*
replayExcursion reads measurements from the catalog for one excursion record's
tick range, runs them through the frozen grid, and trains the trie on
ground-truth labels.

The rep parameter controls the random A-offset: on rep 0 we use the original
precursor start, on subsequent reps we pick a random point between precursor
start and B (anchor), making the model robust to different observation
entry points.
*/
func (training *Training) replayExcursion(
	ctx context.Context,
	excursion tables.ExcursionRecord,
	rep int,
) {
	if !training.grid.Settled {
		return
	}

	// Read measurements for this excursion's tick range from the catalog
	fragments := training.collectExcursionFragments(excursion)

	if len(fragments) == 0 {
		return
	}

	// Determine effective A offset
	aStart := excursion.PrecursorStartTick
	if rep > 0 && excursion.AnchorTick > aStart+10 {
		// Random offset: pick a point between A and B, leaving at least 10 ticks
		spread := excursion.AnchorTick - aStart - 10
		if spread > 0 {
			aStart = aStart + rand.Int64N(spread)
		}
	}

	// Determine ground-truth action class for the excursion type
	var entryAction, exitAction []byte
	switch excursion.Direction {
	case "upward", "upper":
		if excursion.ClearsFriction {
			entryAction = []byte(ActionEnter)
		}
		exitAction = []byte(ActionExit)
	case "downward", "lower":
		exitAction = []byte(ActionExit)
	}

	// Build region tokens and train
	var localPriors [][]byte

	for _, frag := range fragments {
		if frag.SeqIdx < aStart {
			continue
		}

		lit := training.grid.LitRegions(frag, 3)
		if len(lit) == 0 {
			continue
		}

		if len(localPriors) > 0 && bytes.Equal(localPriors[len(localPriors)-1], lit) {
			// Do not append duplicate consecutive states
		} else {
			if len(localPriors) >= 3 {
				localPriors = localPriors[1:]
			}
			localPriors = append(localPriors, lit)
		}

		if len(localPriors) < 1 {
			continue
		}

		// Build temporal signature
		var sig []byte
		for idx := 0; idx < len(localPriors)-1; idx++ {
			sig = append(sig, localPriors[idx]...)
			sig = append(sig, 0x00)
		}
		sig = append(sig, localPriors[len(localPriors)-1]...)

		if len(sig) == 0 {
			continue
		}

		// Determine ground-truth label for this tick
		tick := frag.SeqIdx
		var targetClass []byte

		switch {
		case tick == excursion.AnchorTick:
			// Exact ignition point: learn the prefix that got us here
			if entryAction != nil {
				targetClass = entryAction
			} else if excursion.Direction == "downward" || excursion.Direction == "lower" {
				targetClass = []byte(ActionExit)
			} else {
				targetClass = []byte(ActionWait)
			}
		case exitAction != nil && excursion.ExitTick > 0 && tick == excursion.ExitTick:
			// Exact exhaustion point: learn the prefix that got us here
			targetClass = exitAction
		}

		// Only insert if we are explicitly on the event tick
		if len(targetClass) > 0 {
			training.trie.Insert(sig, targetClass)
			training.decisions.Add(1)
		}
	}

	// Emit training visualization via tee
	if training.tee != nil && len(fragments) > 0 {
		training.emitTrainingViz(excursion, fragments, aStart)
	}
}

/*
collectExcursionFragments gathers measurement fragments for one excursion
from the in-memory fragment buffer. The buffer is populated by ExcursionLearn
from the live StoreTee drain.
*/
func (training *Training) collectExcursionFragments(
	record tables.ExcursionRecord,
) []*data.Measurement[float64] {
	training.mutex.Lock()
	defer training.mutex.Unlock()

	var excursionFrags []*data.Measurement[float64]

	for _, frag := range training.fragments {
		if frag == nil {
			continue
		}

		if frag.Label == record.Symbol &&
			frag.SeqIdx >= record.PrecursorStartTick &&
			frag.SeqIdx <= record.PostEndTick {
			excursionFrags = append(excursionFrags, frag)
		}
	}

	return excursionFrags
}

/*
emitTrainingViz sends training episode visualization data through the tee
for the learning dashboard. Shows the tape fragment with A, B, C markers
and ENTER/EXIT prediction markers.
*/
func (training *Training) emitTrainingViz(
	record tables.ExcursionRecord,
	fragments []*data.Measurement[float64],
	aStart int64,
) {
	var targetAction string
	var excursionTypeVal float64

	switch record.Direction {
	case "upward", "upper":
		excursionTypeVal = 1
		if record.ClearsFriction {
			targetAction = ActionEnter
		} else {
			targetAction = ActionWait
		}
	case "downward", "lower":
		excursionTypeVal = 2
		targetAction = ActionExit
	case "flat":
		excursionTypeVal = 4
		targetAction = ActionWait
	default:
		excursionTypeVal = 3
		targetAction = ActionWait
	}

	var delayedTargetVal float64
	switch targetAction {
	case ActionEnter:
		delayedTargetVal = 1
	case ActionExit:
		delayedTargetVal = 2
	}

	for _, frag := range fragments {
		if frag.SeqIdx < aStart {
			continue
		}

		uiMsg := frag.Clone()
		uiMsg.Source = "training"
		uiMsg.Label = record.Symbol

		if uiMsg.Metrics == nil {
			uiMsg.Metrics = make(map[string]data.Metric[float64])
		}
		if uiMsg.Provenance == nil {
			uiMsg.Provenance = make(map[string]string)
		}
		if uiMsg.Metadata == nil {
			uiMsg.Metadata = make(map[string]string)
		}

		setMetric(uiMsg, "action", delayedTargetVal)
		setMetric(uiMsg, "excursion_type", excursionTypeVal)
		setMetric(uiMsg, "delayed_target", delayedTargetVal)
		setMetric(uiMsg, "iteration", float64(training.iterations.Load()))

		// Markers for the UI visualization
		uiMsg.Provenance["excursion_start"] = strconv.FormatInt(aStart, 10)
		uiMsg.Provenance["excursion_ignition"] = strconv.FormatInt(record.AnchorTick, 10)
		uiMsg.Provenance["excursion_end"] = strconv.FormatInt(record.ExitTick, 10)

		training.emitMetrics(uiMsg, targetAction)
		training.tee.Push(uiMsg)
	}
}

// ExcursionLearn is the learn callback for catalog.Drain (Iceberg). It receives
// StoreTee-tagged measurements carrying excursion metadata.
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

	training.mutex.Lock()
	training.fragments = append(training.fragments, measurement.Clone())
	training.mutex.Unlock()

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
	if metaVal := measurement.Metadata["excursion_end"]; metaVal != "" {
		endTick, _ = strconv.ParseInt(metaVal, 10, 64)
	}

	clearsFriction := measurement.Metadata["excursion_clears_friction"] == "true"
	if measurement.Metadata["excursion_profit"] != "" {
		if profit, _ := strconv.ParseFloat(measurement.Metadata["excursion_profit"], 64); profit > 0 {
			if excursionDirection == "upper" || measurement.Metadata["excursion_category"] == "upper_profitable" {
				clearsFriction = true
			}
		}
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
	record.ClearsFriction = clearsFriction
	record.ObservationCount = measurement.SeqIdx - startTick + 1
	record.Status = "completed"

	select {
	case training.learnQueue <- record:
	default:
		errnie.Error(errnie.Err(errnie.Internal, "training: learn queue full, dropping excursion", nil))
	}

	return []tables.ExcursionRecord{record}, nil
}

func (training *Training) trainOnExcursion(record tables.ExcursionRecord) {
	if !training.grid.Settled && len(training.grid.Metrics) > 0 {
		training.grid.ForceSettle()
	}

	if !training.grid.Settled {
		return
	}

	fragments := training.collectExcursionFragments(record)

	if len(fragments) == 0 {
		return
	}

	// Build region tokens from the excursion fragments
	var localPriors [][]byte

	for _, frag := range fragments {
		if frag.SeqIdx > record.AnchorTick {
			break
		}
		lit := training.grid.LitRegions(frag, 3)
		if len(lit) > 0 {
			if len(localPriors) > 0 && bytes.Equal(localPriors[len(localPriors)-1], lit) {
				// Do not append duplicate consecutive states
			} else {
				if len(localPriors) >= 3 {
					localPriors = localPriors[1:]
				}
				localPriors = append(localPriors, lit)
			}
		}
	}

	var token []byte
	if len(localPriors) > 0 {
		var sig []byte
		for i := 0; i < len(localPriors)-1; i++ {
			sig = append(sig, localPriors[i]...)
			sig = append(sig, 0x00)
		}
		sig = append(sig, localPriors[len(localPriors)-1]...)
		token = sig
	}

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
		} else {
			targetClass = []byte(ActionWait)
		}
	case "downward", "lower":
		targetClass = []byte(ActionExit)
	case "flat":
		targetClass = []byte(ActionWait)
	default:
		targetClass = []byte(ActionWait)
	}

	if len(targetClass) > 0 {
		training.trie.Insert(token, targetClass)
		training.decisions.Add(1)
		training.iterations.Add(1)
	}

	// Prune old fragments to prevent unbounded growth
	training.mutex.Lock()
	var survivingFrags []*data.Measurement[float64]
	for _, frag := range training.fragments {
		if frag != nil && frag.SeqIdx >= record.PrecursorStartTick-1000 {
			survivingFrags = append(survivingFrags, frag)
		}
	}
	training.fragments = survivingFrags
	training.mutex.Unlock()

	if err := training.SaveCheckpoint(); err != nil {
		errnie.Error(err)
	}
}

func (training *Training) RecordTradeResult(returnFraction float64) {
	training.RecordPositionResult("", returnFraction)
}

func (training *Training) RecordPositionResult(symbol string, returnFraction float64) {
	training.totalTrades.Add(1)
	if returnFraction > 0 {
		training.wins.Add(1)
	}

	// Refine the model based on paper trading outcome
	if training.stageCode.Load() >= StageForwardPaper {
		training.mutex.RLock()
		priorsSnapshot := make([][]byte, len(training.priors))
		copy(priorsSnapshot, training.priors)
		training.mutex.RUnlock()

		if len(priorsSnapshot) > 0 {
			var sig []byte
			for idx, prior := range priorsSnapshot {
				sig = append(sig, prior...)
				if idx < len(priorsSnapshot)-1 {
					sig = append(sig, 0x00)
				}
			}

			if len(sig) > 0 {
				if returnFraction > 0 {
					// Winning trade: reinforce the enter action
					training.trie.Insert(sig, []byte(ActionEnter))
				} else {
					// Losing trade: the entry was wrong, reinforce wait
					training.trie.Insert(sig, []byte(ActionWait))
				}
			}
		}
	}
}

var checkpointMu sync.Mutex

func (training *Training) SaveCheckpoint() error {
	checkpointMu.Lock()
	defer checkpointMu.Unlock()

	file, err := os.Create("grid_checkpoint.json")
	if err != nil {
		return errnie.Error(err)
	}
	defer file.Close()

	err = json.NewEncoder(file).Encode(struct {
		Grid *store.Grid  `json:"grid"`
		Trie *store.Radix `json:"trie"`
	}{
		Grid: training.grid,
		Trie: training.trie,
	})

	if err != nil {
		return errnie.Error(err)
	}

	return nil
}

func (training *Training) LoadCheckpoint() error {
	checkpointMu.Lock()
	defer checkpointMu.Unlock()

	payload, err := os.ReadFile("grid_checkpoint.json")
	if err != nil {
		return err
	}

	state := struct {
		Grid *store.Grid  `json:"grid"`
		Trie *store.Radix `json:"trie"`
	}{
		Grid: training.grid,
		Trie: training.trie,
	}

	if err := json.Unmarshal(payload, &state); err != nil {
		return errnie.Error(err)
	}

	if training.grid.Settled {
		training.Transition(runtime.READY)
		training.stageCode.Store(StageHistoricalTraining)
	}

	return nil
}

func (training *Training) CognitionTree() cognition.CognitionTreeExport {
	training.mutex.RLock()
	defer training.mutex.RUnlock()

	rootNode := &cognition.TrieNodeJSON{
		ID:          "root",
		TokenPrefix: "ROOT",
		Probability: 1.0,
		State:       "EVALUATED",
	}

	tree := training.trie.Tree()
	if tree != nil && tree.Len() > 0 {
		nodeCount := 0
		iterator := tree.Root().Iterator()
		for key, val, ok := iterator.Next(); ok; key, val, ok = iterator.Next() {
			if nodeCount > 300 {
				break // Prevent massive UI payload crashes
			}
			action := string(val)
			if action == "" {
				continue
			}

			var regionTokens []string
			parts := bytes.Split(key, []byte{0x00})
			for _, part := range parts {
				if len(part) == 0 {
					continue
				}
				var strTokens []string
				for _, b := range part {
					strTokens = append(strTokens, fmt.Sprintf("%02X", b))
				}
				regionTokens = append(regionTokens, strings.Join(strTokens, ""))
			}

			if len(regionTokens) == 0 {
				continue
			}

			currNode := rootNode
			var pathSoFar strings.Builder
			pathSoFar.WriteString("root")

			for idx, regToken := range regionTokens {
				isLast := idx == len(regionTokens)-1
				nodeAction := "WAIT"
				nodeState := "EVALUATED"

				if isLast {
					nodeAction = strings.ToUpper(action)
					nodeState = "POLICY CHOICE"
				}

				pathSoFar.WriteString("/")
				pathSoFar.WriteString(regToken)

				var foundChild *cognition.TrieNodeJSON
				for _, child := range currNode.Children {
					if len(child.Tokens) > 0 && child.Tokens[0] == regToken {
						foundChild = child
						break
					}
				}

				if foundChild == nil {
					foundChild = &cognition.TrieNodeJSON{
						ID:          pathSoFar.String(),
						TokenPrefix: nodeAction,
						Probability: 1.0,
						Count:       1,
						Tokens:      []string{regToken},
						State:       nodeState,
					}
					currNode.Children = append(currNode.Children, foundChild)
					nodeCount++
				} else if isLast {
					foundChild.TokenPrefix = nodeAction
					foundChild.State = nodeState
					foundChild.Count++
				}

				currNode = foundChild
			}
		}
	}

	return cognition.CognitionTreeExport{
		Root:     rootNode,
		Branches: []cognition.TrieBranchJSON{},
		Feasible: []cognition.FeasibleActionJSON{},
	}
}

func (training *Training) emitMetrics(measurement *data.Measurement[float64], action string) {
	if measurement.Metrics == nil {
		measurement.Metrics = make(map[string]data.Metric[float64])
	}
	if measurement.Provenance == nil {
		measurement.Provenance = make(map[string]string)
	}

	stage := training.stageCode.Load()
	switch stage {
	case StageGridSettling:
		measurement.Provenance["stage"] = "GRID SETTLING"
		measurement.Provenance["stage_blocker"] = "awaiting region convergence"
	case StageHistoricalTraining:
		measurement.Provenance["stage"] = "HISTORICAL TRAINING"
		measurement.Provenance["stage_blocker"] = fmt.Sprintf("%d iterations", training.iterations.Load())
	case StageForwardPaper:
		measurement.Provenance["stage"] = "FORWARD PAPER"
		measurement.Provenance["stage_blocker"] = "N/A"
	default:
		measurement.Provenance["stage"] = "SKILL DEMONSTRATED"
		measurement.Provenance["stage_blocker"] = "N/A"
	}

	var actVal float64
	switch action {
	case ActionEnter:
		actVal = 1
	case ActionExit:
		actVal = 2
	}

	setMetric(measurement, "action", actVal)
	setMetric(measurement, "stage_code", float64(stage))
	setMetric(measurement, "steps", float64(training.steps.Load()))
	setMetric(measurement, "decisions", float64(training.decisions.Load()))
	setMetric(measurement, "evaluated", float64(training.evaluated.Load()))
	setMetric(measurement, "confidence", 1.0)
}

func setMetric(m *data.Measurement[float64], name string, val float64) {
	metric := data.NewMetric[float64](name, data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1)
	metric.Raw = val
	m.Metrics[name] = metric
}
