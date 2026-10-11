package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
Trainer discovers the optimal precursor window length using Monte Carlo Tree Search (MCTS)
over historical excursions and continuous sensory tape, constructing an unpruned learned
action tree whose decisions are governed by unanimous matching continuations.
*/
type Trainer struct {
	*runtime.System
	catalog *tables.Catalog
	price   *broker.Price
}

func NewTrainer(
	ctx context.Context,
	catalog *tables.Catalog,
	price *broker.Price,
) *Trainer {
	trainer := &Trainer{
		catalog: catalog,
		price:   price,
	}

	trainer.System = runtime.NewSystem(ctx, "trainer")
	return trainer
}

/*
Generate runs MCTS optimization to discover the optimal precursorTicks,
fits an unpruned action tree on historical evidence, and publishes the
resulting consensus-governed decision paths to S3.
*/
func (trainer *Trainer) Generate(epochs ...int64) error {
	if trainer.price == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] price system is required to evaluate excursions",
			nil,
		))
	}

	excursions, tapeByTick, tapeTicks, err := trainer.loadTrainingData(epochs...)

	if err != nil {
		return errnie.Error(err)
	}

	if len(excursions) == 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] no valid historical excursions found",
			nil,
		))
	}

	grid := store.NewGrid()
	tokensByTick := buildTokenMap(grid, tapeByTick)
	trainExcursions, evalExcursions, evalTicks := splitChronologically(excursions, tapeTicks)

	minSearchTicks, maxSearchTicks := deriveSearchRange(excursions)
	selectedTicks, bestPnL := trainer.optimizePrecursorTicksWithTokens(
		trainExcursions,
		tokensByTick,
		evalExcursions,
		evalTicks,
		minSearchTicks,
		maxSearchTicks,
	)

	errnie.Info(fmt.Sprintf(
		"[training] MCTS selected precursorTicks=%d (replay net return=%.4f) from range [%d, %d]",
		selectedTicks, bestPnL, minSearchTicks, maxSearchTicks,
	))

	finalTrie := trainer.buildCandidateTrie(excursions, tokensByTick, selectedTicks)
	storedCount, validKeys, err := finalTrie.Publish(trainer.Context(), trainer.catalog)

	if err != nil {
		return errnie.Error(err)
	}

	if err := trainer.cleanupStaleBlobs(validKeys); err != nil {
		return errnie.Error(err)
	}

	errnie.Info(fmt.Sprintf(
		"[training] Train complete: %d unpruned action paths published to S3 (enter=%d exit=%d noop=%d)",
		storedCount,
		finalTrie.ActionCount(actionEnter),
		finalTrie.ActionCount(actionExit),
		finalTrie.ActionCount(actionNoop),
	))

	return nil
}

/*
excursionData captures historical excursion facts, prices, and economic returns.
*/
type excursionData struct {
	excursion   *data.Measurement
	symbol      string
	epoch       int64
	startTick   int64
	bTick       int64
	cTick       int64
	startIdx    int64
	bPrice      *decimal.Decimal
	cPrice      *decimal.Decimal
	isUp        bool
	class       string
	gain        float64
	netReturn   float64
	holdSeconds float64
}

func (trainer *Trainer) loadTrainingData(
	epochs ...int64,
) ([]*excursionData, map[int64][]*data.Measurement, []int64, error) {
	byEpoch := make(map[int64][]*data.Measurement)

	for excursion, err := range trainer.catalog.Excursions(trainer.Context(), epochs...) {
		if err != nil {
			return nil, nil, nil, errnie.Error(err)
		}

		if excursion != nil {
			byEpoch[excursion.Epoch] = append(byEpoch[excursion.Epoch], excursion)
		}
	}

	targetEpochs := sortedEpochKeys(byEpoch)
	tapeByTick := make(map[int64][]*data.Measurement)
	allExcursions := make([]*excursionData, 0)

	for _, epoch := range targetEpochs {
		epochExcursions := byEpoch[epoch]
		minTick, maxTick := excursionTickSpan(epochExcursions)

		for measurement, tapeErr := range trainer.catalog.SignalLogic(
			trainer.Context(),
			epoch,
			"",
			minTick,
			maxTick,
			tables.SensorySources...,
		) {
			if tapeErr != nil {
				return nil, nil, nil, errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("[training] tape read error in epoch %d", epoch),
					tapeErr,
				))
			}

			if measurement != nil {
				tapeByTick[measurement.Tick] = append(tapeByTick[measurement.Tick], measurement)
			}
		}

		for _, excursion := range epochExcursions {
			record, evalErr := trainer.parseExcursionData(excursion)

			if evalErr != nil {
				return nil, nil, nil, errnie.Error(evalErr)
			}

			if record != nil {
				allExcursions = append(allExcursions, record)
			}
		}
	}

	ticks := make([]int64, 0, len(tapeByTick))

	for tick := range tapeByTick {
		ticks = append(ticks, tick)
	}

	slices.Sort(ticks)

	slices.SortFunc(allExcursions, func(left, right *excursionData) int {
		if left.bTick < right.bTick {
			return -1
		}

		if left.bTick > right.bTick {
			return 1
		}

		return 0
	})

	return allExcursions, tapeByTick, ticks, nil
}

func (trainer *Trainer) parseExcursionData(excursion *data.Measurement) (*excursionData, error) {
	bPrice, cPrice, priceErr := tables.DetectionPrices(excursion)

	if priceErr != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[training] unable to extract detection prices for %s", excursion.Label),
			priceErr,
		))
	}

	if bPrice == nil || cPrice == nil || bPrice.Sign() <= 0 || cPrice.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[training] invalid detection prices for %s", excursion.Label),
			nil,
		))
	}

	symbol := excursion.Label
	pnl, cost, roundTripErr := trainer.price.RoundTrip(symbol, bPrice, cPrice)

	if roundTripErr != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[training] round trip cannot be priced for %s: %s -> %s", symbol, bPrice, cPrice),
			roundTripErr,
		))
	}

	if cost == nil || cost.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[training] round-trip cost is non-positive for %s", symbol),
			nil,
		))
	}

	startTick := int64(data.Pull(excursion.Read("start_tick")).Metric.Raw)
	bTick := int64(data.Pull(excursion.Read("b_tick")).Metric.Raw)
	cTick := int64(data.Pull(excursion.Read("c_tick")).Metric.Raw)

	if bTick <= startTick {
		return nil, nil
	}

	startIdx := int64(data.Pull(excursion.Read("start_idx")).Metric.Raw)
	class := excursionClass(excursion)
	isUp := strings.EqualFold(class, excursionUp)
	gain := cPrice.Sub(bPrice).Div(bPrice).Float64()
	netReturn := pnl.Div(cost).Float64()
	holdSeconds := excursionHoldSeconds(excursion)

	return &excursionData{
		excursion:   excursion,
		symbol:      symbol,
		epoch:       excursion.Epoch,
		startTick:   startTick,
		bTick:       bTick,
		cTick:       cTick,
		startIdx:    startIdx,
		bPrice:      bPrice,
		cPrice:      cPrice,
		isUp:        isUp,
		class:       class,
		gain:        gain,
		netReturn:   netReturn,
		holdSeconds: holdSeconds,
	}, nil
}

func splitChronologically(
	excursions []*excursionData,
	tapeTicks []int64,
) ([]*excursionData, []*excursionData, []int64) {
	if len(excursions) < 2 {
		return excursions, excursions, tapeTicks
	}

	splitIndex := int(float64(len(excursions)) * 0.70)

	if splitIndex < 1 {
		splitIndex = 1
	}

	if splitIndex >= len(excursions) {
		splitIndex = len(excursions) - 1
	}

	trainExcursions := excursions[:splitIndex]
	evalExcursions := excursions[splitIndex:]
	splitTick := evalExcursions[0].startTick

	evalTicks := make([]int64, 0)

	for _, tick := range tapeTicks {
		if tick >= splitTick {
			evalTicks = append(evalTicks, tick)
		}
	}

	if len(evalTicks) == 0 {
		evalTicks = tapeTicks
	}

	return trainExcursions, evalExcursions, evalTicks
}

func deriveSearchRange(excursions []*excursionData) (int64, int64) {
	minSpan := int64(0)
	maxSpan := int64(0)

	for _, exc := range excursions {
		span := exc.bTick - exc.startTick

		if span <= 0 {
			continue
		}

		if minSpan == 0 || span < minSpan {
			minSpan = span
		}

		if span > maxSpan {
			maxSpan = span
		}
	}

	if minSpan == 0 {
		minSpan = 1
	}

	if maxSpan <= minSpan {
		maxSpan = minSpan + 8
	}

	return minSpan, maxSpan
}

func (trainer *Trainer) buildCandidateTrie(
	records []*excursionData,
	tokensByTick map[int64]string,
	precursorTicks int64,
) *precursorTrie {
	trie := newPrecursorTrie()

	for _, record := range records {
		windowStart := record.bTick - precursorTicks

		if windowStart < 0 {
			windowStart = 0
		}

		entryTokens := sliceTokens(
			tokensByTick,
			windowStart,
			record.bTick,
			false,
		)

		if len(entryTokens) == 0 {
			continue
		}

		entryAction := actionNoop

		if record.isUp {
			entryAction = actionEnter
		}

		trie.Insert(
			entryTokens,
			entryAction,
			record.netReturn,
			record.gain,
			record.holdSeconds,
		)

		if record.isUp && record.cTick > record.bTick {
			exitTokens := sliceTokens(
				tokensByTick,
				record.bTick,
				record.cTick,
				true,
			)

			if len(exitTokens) > 0 {
				trie.Insert(
					exitTokens,
					actionExit,
					record.netReturn,
					record.gain,
					record.holdSeconds,
				)
			}
		}
	}

	return trie
}

func sliceTokens(
	tokensByTick map[int64]string,
	startTick, endTick int64,
	endInclusive bool,
) []string {
	tokens := make([]string, 0)
	limit := endTick

	if endInclusive {
		limit = endTick + 1
	}

	for tick := startTick; tick < limit; tick++ {
		token, exists := tokensByTick[tick]

		if !exists || token == "" {
			continue
		}

		if len(tokens) == 0 || tokens[len(tokens)-1] != token {
			tokens = append(tokens, token)
		}
	}

	return tokens
}

func buildTokenMap(
	grid *store.Grid,
	tapeByTick map[int64][]*data.Measurement,
) map[int64]string {
	tokensByTick := make(map[int64]string, len(tapeByTick))

	for tick, signals := range tapeByTick {
		if len(signals) == 0 {
			continue
		}

		frame := data.NewMeasurement(signals[0].Epoch, signals[0].Label, "training", 0, tick)
		frame.At = signals[0].At
		frame.From = signals[0].From

		if frame.From.IsZero() && !frame.At.IsZero() {
			frame.From = frame.At
		}

		frame.Peers(signals...)
		frame.Write()
		tokensByTick[tick] = string(grid.Observe(frame))
	}

	return tokensByTick
}

/*
tokenizeTape scans ticks in [startTick, endTick), evaluates each frame via grid,
and collapses consecutive duplicate region tokens into single transitions.
*/
func tokenizeTape(
	grid *store.Grid,
	tapeByTick map[int64][]*data.Measurement,
	startTick, endTick int64,
	epoch int64,
	symbol string,
	startIdx int64,
	endInclusive bool,
) []string {
	tokensByTick := buildTokenMap(grid, tapeByTick)
	return sliceTokens(tokensByTick, startTick, endTick, endInclusive)
}

/*
replayOutcome records predictive and economic performance across holdout replay.
*/
type replayOutcome struct {
	candidateTicks int64
	correctEntries int
	falseEntries   int
	missedOpps     int
	abstentions    int
	totalLeadTicks int64
	avgLeadTicks   float64
	netPnL         float64
	grossProfit    float64
	grossLoss      float64
}

func (trainer *Trainer) evaluateReplay(
	candidateTrie *precursorTrie,
	tokensByTick map[int64]string,
	evalTicks []int64,
	evalExcursions []*excursionData,
	candidateTicks int64,
) replayOutcome {
	outcome := replayOutcome{
		candidateTicks: candidateTicks,
	}

	inPosition := false
	entryTick := int64(0)
	var activeExcursion *excursionData
	runningTokens := make([]string, 0)

	for _, tick := range evalTicks {
		token, exists := tokensByTick[tick]

		if !exists || token == "" {
			continue
		}

		if len(runningTokens) == 0 || runningTokens[len(runningTokens)-1] != token {
			runningTokens = append(runningTokens, token)
		}

		action := candidateTrie.Consensus(runningTokens)

		if !inPosition {
			if action == actionEnter {
				inPosition = true
				entryTick = tick

				var target *excursionData

				for _, exc := range evalExcursions {
					if exc.bTick >= tick {
						target = exc
						break
					}
				}

				if target != nil && target.isUp && target.cTick > tick {
					outcome.correctEntries++
					lead := target.bTick - tick
					outcome.totalLeadTicks += lead
					activeExcursion = target
				}

				if target == nil || !target.isUp {
					outcome.falseEntries++
					activeExcursion = nil
					outcome.netPnL -= 0.0052
					outcome.grossLoss += 0.0052
				}
			}

			if action != actionEnter {
				outcome.abstentions++
			}
		}

		if inPosition {
			shouldExit := action == actionExit

			if activeExcursion != nil && tick >= activeExcursion.cTick {
				shouldExit = true
			}

			if activeExcursion == nil && tick >= entryTick+candidateTicks {
				shouldExit = true
			}

			if shouldExit {
				inPosition = false

				if activeExcursion != nil && activeExcursion.isUp {
					ret := activeExcursion.netReturn
					outcome.netPnL += ret
					outcome.grossProfit += ret
				}

				activeExcursion = nil
			}
		}
	}

	for _, exc := range evalExcursions {
		if exc.isUp && exc.bTick > entryTick && !inPosition {
			outcome.missedOpps++
		}
	}

	if outcome.correctEntries > 0 {
		outcome.avgLeadTicks = float64(outcome.totalLeadTicks) / float64(outcome.correctEntries)
	}

	return outcome
}

/*
mctsNode tracks a precursor tick interval explored during MCTS optimization.
*/
type mctsNode struct {
	minTicks int64
	maxTicks int64
	visits   int
	totalPnL float64
	bestPnL  float64
	children []*mctsNode
	parent   *mctsNode
}

func newMCTSNode(minTicks, maxTicks int64, parent *mctsNode) *mctsNode {
	return &mctsNode{
		minTicks: minTicks,
		maxTicks: maxTicks,
		parent:   parent,
		children: make([]*mctsNode, 0),
		bestPnL:  -math.MaxFloat64,
	}
}

func (node *mctsNode) ucb1Score(explorationConstant float64) float64 {
	if node.visits == 0 {
		return math.MaxFloat64
	}

	parentVisits := float64(node.visits)

	if node.parent != nil && node.parent.visits > 0 {
		parentVisits = float64(node.parent.visits)
	}

	exploitation := node.totalPnL / float64(node.visits)
	exploration := explorationConstant * math.Sqrt(math.Log(parentVisits)/float64(node.visits))
	return exploitation + exploration
}

func (node *mctsNode) selectChild(explorationConstant float64) *mctsNode {
	var bestChild *mctsNode
	bestScore := -math.MaxFloat64

	for _, child := range node.children {
		score := child.ucb1Score(explorationConstant)

		if score > bestScore {
			bestScore = score
			bestChild = child
		}
	}

	return bestChild
}

func (node *mctsNode) expand() {
	if node.maxTicks <= node.minTicks {
		return
	}

	if node.maxTicks-node.minTicks == 1 {
		node.children = append(node.children,
			newMCTSNode(node.minTicks, node.minTicks, node),
			newMCTSNode(node.maxTicks, node.maxTicks, node),
		)
		return
	}

	mid := (node.minTicks + node.maxTicks) / 2
	node.children = append(node.children,
		newMCTSNode(node.minTicks, mid, node),
		newMCTSNode(mid+1, node.maxTicks, node),
	)
}

func (node *mctsNode) bestCandidateTicks() (int64, float64) {
	if len(node.children) == 0 {
		avg := 0.0

		if node.visits > 0 {
			avg = node.totalPnL / float64(node.visits)
		}

		return (node.minTicks + node.maxTicks) / 2, avg
	}

	var bestChild *mctsNode
	bestVisits := -1
	bestAvg := -math.MaxFloat64

	for _, child := range node.children {
		avg := -math.MaxFloat64

		if child.visits > 0 {
			avg = child.totalPnL / float64(child.visits)
		}

		if child.visits > bestVisits || (child.visits == bestVisits && avg > bestAvg) {
			bestVisits = child.visits
			bestAvg = avg
			bestChild = child
		}
	}

	if bestChild == nil {
		return (node.minTicks + node.maxTicks) / 2, 0.0
	}

	return bestChild.bestCandidateTicks()
}

func (trainer *Trainer) optimizePrecursorTicks(
	trainExcursions []*excursionData,
	tapeByTick map[int64][]*data.Measurement,
	evalExcursions []*excursionData,
	evalTicks []int64,
	grid *store.Grid,
	minSearchTicks, maxSearchTicks int64,
) (int64, float64) {
	tokensByTick := buildTokenMap(grid, tapeByTick)

	return trainer.optimizePrecursorTicksWithTokens(
		trainExcursions,
		tokensByTick,
		evalExcursions,
		evalTicks,
		minSearchTicks,
		maxSearchTicks,
	)
}

func (trainer *Trainer) optimizePrecursorTicksWithTokens(
	trainExcursions []*excursionData,
	tokensByTick map[int64]string,
	evalExcursions []*excursionData,
	evalTicks []int64,
	minSearchTicks, maxSearchTicks int64,
) (int64, float64) {
	root := newMCTSNode(minSearchTicks, maxSearchTicks, nil)
	iterations := int(maxSearchTicks-minSearchTicks+1) * 4

	if iterations < 16 {
		iterations = 16
	}

	if iterations > 64 {
		iterations = 64
	}

	explorationConstant := math.Sqrt(2)

	for iteration := 0; iteration < iterations; iteration++ {
		current := root

		for len(current.children) > 0 {
			current = current.selectChild(explorationConstant)
		}

		if current.visits > 0 && current.maxTicks > current.minTicks {
			current.expand()
			current = current.children[0]
		}

		candidateTicks := (current.minTicks + current.maxTicks) / 2
		candidateTrie := trainer.buildCandidateTrie(trainExcursions, tokensByTick, candidateTicks)
		outcome := trainer.evaluateReplay(candidateTrie, tokensByTick, evalTicks, evalExcursions, candidateTicks)

		reward := outcome.netPnL

		for node := current; node != nil; node = node.parent {
			node.visits++
			node.totalPnL += reward

			if reward > node.bestPnL {
				node.bestPnL = reward
			}
		}
	}

	return root.bestCandidateTicks()
}

/*
precursorTrie maintains unpruned precursor paths and their terminal actions.
*/
type precursorTrie struct {
	root *precursorNode
}

func newPrecursorTrie() *precursorTrie {
	return &precursorTrie{
		root: newPrecursorNode("", "", 0, nil),
	}
}

func (trie *precursorTrie) Insert(
	tokens []string,
	action string,
	netReturn, gain, holdSeconds float64,
) {
	current := trie.root

	for depthIndex, token := range tokens {
		depth := depthIndex + 1
		forwardPath := strings.Join(tokens[:depth], "/")

		child, exists := current.children[token]

		if !exists {
			child = newPrecursorNode(token, forwardPath, depth, current)
			current.children[token] = child
		}

		current = child
		current.visits++
		current.totalReturn += netReturn
		current.totalGain += gain
		current.totalHoldSecs += holdSeconds
	}

	current.action = action
	current.actions[action]++
}

/*
Consensus returns action only when ALL matching stored continuations in the
subtree agree on that action. Disagreement or unmapped prefixes return "".
*/
func (trie *precursorTrie) Consensus(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}

	current := trie.root

	for _, token := range tokens {
		child, exists := current.children[token]

		if !exists {
			return ""
		}

		current = child
	}

	actions := make(map[string]struct{})
	collectSubtreeActions(current, actions)

	if len(actions) != 1 {
		return ""
	}

	for singleAction := range actions {
		return singleAction
	}

	return ""
}

func collectSubtreeActions(node *precursorNode, actions map[string]struct{}) {
	for actionKey, count := range node.actions {
		if count > 0 {
			actions[actionKey] = struct{}{}
		}
	}

	for _, child := range node.children {
		collectSubtreeActions(child, actions)
	}
}

func (trie *precursorTrie) ActionCount(targetAction string) int {
	count := 0

	var walk func(node *precursorNode)

	walk = func(node *precursorNode) {
		if node.actions[targetAction] > 0 && node.path != "" {
			count += node.actions[targetAction]
		}

		for _, child := range node.children {
			walk(child)
		}
	}

	walk(trie.root)
	return count
}

func (trie *precursorTrie) Publish(
	ctx context.Context,
	catalog *tables.Catalog,
) (int, map[string]struct{}, error) {
	validKeys := make(map[string]struct{})
	storedCount := 0

	var storeAction func(node *precursorNode) error

	storeAction = func(node *precursorNode) error {
		if node.path != "" {
			for actionName, count := range node.actions {
				if count == 0 {
					continue
				}

				stats := node.stats()
				body, marshalErr := json.Marshal(stats)

				if marshalErr != nil {
					return errnie.Error(errnie.Err(
						errnie.Internal,
						"[training] failed to marshal pathStats for "+node.path,
						marshalErr,
					))
				}

				blobKey := node.path + "/" + actionName
				validKeys[blobKey] = struct{}{}

				putErr := catalog.PutBlob(ctx, blobKey, body)

				if putErr != nil && !errors.Is(putErr, tables.ErrBlobStorageUnconfigured) {
					return errnie.Error(errnie.Err(
						errnie.IO,
						"[training] failed to put blob: "+blobKey,
						putErr,
					))
				}

				storedCount++
			}
		}

		for _, child := range node.children {
			if err := storeAction(child); err != nil {
				return err
			}
		}

		return nil
	}

	if err := storeAction(trie.root); err != nil {
		return 0, nil, errnie.Error(err)
	}

	return storedCount, validKeys, nil
}

type precursorNode struct {
	token         string
	path          string
	depth         int
	visits        int
	totalReturn   float64
	totalGain     float64
	totalHoldSecs float64
	children      map[string]*precursorNode
	parent        *precursorNode
	action        string
	actions       map[string]int
}

func newPrecursorNode(
	token, path string, depth int, parent *precursorNode,
) *precursorNode {
	return &precursorNode{
		token:    token,
		path:     path,
		depth:    depth,
		parent:   parent,
		children: make(map[string]*precursorNode),
		actions:  make(map[string]int),
	}
}

func (node *precursorNode) stats() pathStats {
	avgGain := 0.0
	avgHold := 0.0
	meanReturn := 0.0

	if node.visits > 0 {
		avgGain = node.totalGain / float64(node.visits)
		avgHold = node.totalHoldSecs / float64(node.visits)
		meanReturn = node.totalReturn / float64(node.visits)
	}

	winRate := 0.0

	if enters, ok := node.actions[actionEnter]; ok && node.visits > 0 {
		winRate = float64(enters) / float64(node.visits)
	}

	return pathStats{
		Gain:        &avgGain,
		HoldSeconds: &avgHold,
		Visits:      &node.visits,
		WinRate:     &winRate,
		MeanReward:  &meanReturn,
	}
}

func excursionTickSpan(excursions []*data.Measurement) (int64, int64) {
	minTick := int64(0)
	maxTick := int64(0)

	for _, excursion := range excursions {
		start := int64(data.Pull(excursion.Read("start_tick")).Metric.Raw)
		end := int64(data.Pull(excursion.Read("c_tick")).Metric.Raw)

		if minTick == 0 || (start > 0 && start < minTick) {
			minTick = start
		}

		if end > maxTick {
			maxTick = end
		}
	}

	return minTick, maxTick
}

func excursionClass(excursion *data.Measurement) string {
	class := excursion.Meta("type")

	if class == "" {
		class = excursion.Meta("class")
	}

	if class == "" {
		class = excursion.Meta("direction")
	}

	return class
}

func excursionHoldSeconds(excursion *data.Measurement) float64 {
	if !excursion.At.IsZero() && !excursion.From.IsZero() && excursion.At.After(excursion.From) {
		return excursion.At.Sub(excursion.From).Seconds()
	}

	return 0.0
}

func sortedEpochKeys(byEpoch map[int64][]*data.Measurement) []int64 {
	keys := make([]int64, 0, len(byEpoch))

	for epoch := range byEpoch {
		keys = append(keys, epoch)
	}

	slices.Sort(keys)
	return keys
}

func (trainer *Trainer) cleanupStaleBlobs(validKeys map[string]struct{}) error {
	for existingKey, listErr := range trainer.catalog.ListBlobs(trainer.Context(), "") {
		if listErr != nil {
			if errors.Is(listErr, tables.ErrBlobStorageUnconfigured) {
				return nil
			}

			return errnie.Error(errnie.Err(
				errnie.IO,
				"[training] failed to list blobs during cleanup",
				listErr,
			))
		}

		if !strings.HasSuffix(existingKey, actionEnter) &&
			!strings.HasSuffix(existingKey, actionExit) &&
			!strings.HasSuffix(existingKey, actionNoop) {
			continue
		}

		if _, exists := validKeys[existingKey]; !exists {
			deleteErr := trainer.catalog.DeleteBlob(trainer.Context(), existingKey)

			if deleteErr != nil && !errors.Is(deleteErr, tables.ErrBlobStorageUnconfigured) {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"[training] failed to delete stale blob: "+existingKey,
					deleteErr,
				))
			}
		}
	}

	return nil
}
