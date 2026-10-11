package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	stdruntime "runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	symbols map[string]struct{}
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

type progressTracker struct {
	label     string
	total     int64
	startTime time.Time
	lastLog   time.Time
	interval  time.Duration
	completed bool
	mutex     sync.Mutex
}

func newProgressTracker(label string, total int64, interval time.Duration) *progressTracker {
	now := time.Now()
	return &progressTracker{
		label:     label,
		total:     total,
		startTime: now,
		lastLog:   now,
		interval:  interval,
	}
}

func (tracker *progressTracker) Step(completed int64, extra ...string) {
	if tracker == nil || tracker.total <= 0 {
		return
	}

	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()

	if tracker.completed {
		return
	}

	now := time.Now()
	if completed < tracker.total && now.Sub(tracker.lastLog) < tracker.interval {
		return
	}

	if completed >= tracker.total {
		tracker.completed = true
		completed = tracker.total
	}

	tracker.lastLog = now
	elapsed := now.Sub(tracker.startTime)
	percent := float64(completed) / float64(tracker.total) * 100.0

	var etaStr string
	if completed > 0 && completed < tracker.total {
		rate := float64(completed) / elapsed.Seconds()
		if rate > 0 {
			remainingSec := float64(tracker.total-completed) / rate
			etaStr = fmt.Sprintf(" | ETA: %s", (time.Duration(remainingSec) * time.Second).Round(time.Second))
		}
	}

	extraStr := ""
	if len(extra) > 0 && extra[0] != "" {
		extraStr = " | " + extra[0]
	}

	errnie.Info(fmt.Sprintf(
		"[training] [%s] %d / %d (%.1f%%) | elapsed: %s%s%s",
		tracker.label, completed, tracker.total, percent, elapsed.Round(time.Second), etaStr, extraStr,
	))
}

func (trainer *Trainer) SetSymbols(symbols ...string) {
	if trainer == nil {
		return
	}

	if len(symbols) == 0 {
		trainer.symbols = nil
		return
	}

	trainer.symbols = make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		if symbol != "" {
			trainer.symbols[symbol] = struct{}{}
		}
	}
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

	grid := store.NewGrid()
	excursions, tokensByTick, tapeTicks, err := trainer.loadTrainingData(grid, epochs...)

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

type tickAccumulator struct {
	deformation [13]float64
	counts      [13]int
}

func computeToken(acc *tickAccumulator) string {
	halfNormalMean := math.Sqrt(2.0 / math.Pi)
	halfNormalVariance := 1.0 - 2.0/math.Pi

	var winner uint8
	var runnerUp uint8
	var brightness [13]float64

	for regionIdx := uint8(1); regionIdx <= 12; regionIdx++ {
		countVal := float64(acc.counts[regionIdx])

		if countVal == 0 {
			continue
		}

		score := (acc.deformation[regionIdx] - countVal*halfNormalMean) /
			math.Sqrt(countVal*halfNormalVariance)
		brightness[regionIdx] = score

		if winner == 0 || score > brightness[winner] {
			runnerUp = winner
			winner = regionIdx
			continue
		}

		if runnerUp == 0 || score > brightness[runnerUp] {
			runnerUp = regionIdx
		}
	}

	return fmt.Sprintf("R%02d", winner)
}

type epochResult struct {
	epoch      int64
	tokens     map[int64]string
	ticks      []int64
	excursions []*excursionData
	maxTick    int64
	err        error
}

func (trainer *Trainer) loadEpoch(
	grid *store.Grid,
	epoch int64,
	epochExcursions []*data.Measurement,
) epochResult {
	minTick, maxTick := excursionTickSpan(epochExcursions)

	errnie.Info(fmt.Sprintf(
		"[training] reading tape for epoch %d (%d excursions, tick span [%d, %d])",
		epoch, len(epochExcursions), minTick, maxTick,
	))

	accumulators := make(map[int64]*tickAccumulator)
	loadStart := time.Now()
	var measurementCount int64

	lastLog := time.Now()

	for measurement, tapeErr := range trainer.catalog.SignalLogic(
		trainer.Context(),
		epoch,
		"",
		minTick,
		maxTick,
		tables.SensorySources...,
	) {
		if tapeErr != nil {
			return epochResult{
				err: errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("[training] tape read error in epoch %d", epoch),
					tapeErr,
				)),
			}
		}

		if measurement == nil {
			continue
		}

		measurementCount++
		now := time.Now()

		if now.Sub(lastLog) >= 5*time.Second {
			lastLog = now
			elapsed := now.Sub(loadStart)
			rate := float64(measurementCount) / elapsed.Seconds()
			errnie.Info(fmt.Sprintf(
				"[training] [epoch %d tape] %d measurements (%d unique ticks) | elapsed: %s (%.0f meas/s)",
				epoch, measurementCount, len(accumulators), elapsed.Round(time.Second), rate,
			))
		}

		acc, exists := accumulators[measurement.Tick]

		if !exists {
			acc = &tickAccumulator{}
			accumulators[measurement.Tick] = acc
		}

		for metric := range measurement.Read() {
			region := grid.PinRegion(measurement.Source, metric.Key)

			if region == 0 {
				continue
			}

			if !metric.Metric.Standardizable() {
				continue
			}

			acc.deformation[region] += math.Abs(metric.Metric.Standardized)
			acc.counts[region]++
		}
	}

	elapsed := time.Since(loadStart)
	errnie.Info(fmt.Sprintf(
		"[training] [epoch %d tape] complete: %d measurements across %d unique ticks in %s",
		epoch, measurementCount, len(accumulators), elapsed.Round(time.Second),
	))

	tokens := make(map[int64]string, len(accumulators))
	ticks := make([]int64, 0, len(accumulators))

	for tick, acc := range accumulators {
		tokens[tick] = computeToken(acc)
		ticks = append(ticks, tick)
	}

	slices.Sort(ticks)

	excursions := make([]*excursionData, 0, len(epochExcursions))
	numWorkers := max(stdruntime.GOMAXPROCS(0), 1)

	if len(epochExcursions) <= 100 || numWorkers <= 1 {
		for _, excursion := range epochExcursions {
			record, evalErr := trainer.parseExcursionData(excursion)

			if evalErr != nil {
				return epochResult{err: errnie.Error(evalErr)}
			}

			if record != nil {
				excursions = append(excursions, record)
			}
		}
	}

	if len(epochExcursions) > 100 && numWorkers > 1 {
		excursionResults := make([]*excursionData, len(epochExcursions))
		var parseGroup sync.WaitGroup
		parseWorkers := min(numWorkers, 16)
		var parseErr atomic.Pointer[error]

		for workerIdx := range parseWorkers {
			parseGroup.Add(1)

			go func(wID int) {
				defer parseGroup.Done()

				for idx := wID; idx < len(epochExcursions); idx += parseWorkers {
					if parseErr.Load() != nil {
						return
					}

					exc := epochExcursions[idx]
					record, evalErr := trainer.parseExcursionData(exc)

					if evalErr != nil {
						parseErr.Store(&evalErr)
						return
					}

					excursionResults[idx] = record
				}
			}(workerIdx)
		}

		parseGroup.Wait()

		if errPtr := parseErr.Load(); errPtr != nil {
			return epochResult{err: errnie.Error(*errPtr)}
		}

		for _, record := range excursionResults {
			if record != nil {
				excursions = append(excursions, record)
			}
		}
	}

	return epochResult{
		epoch:      epoch,
		tokens:     tokens,
		ticks:      ticks,
		excursions: excursions,
		maxTick:    maxTick,
	}
}

func (trainer *Trainer) loadTrainingData(
	grid *store.Grid,
	epochs ...int64,
) ([]*excursionData, map[int64]string, []int64, error) {
	byEpoch := make(map[int64][]*data.Measurement)

	for excursion, err := range trainer.catalog.Excursions(trainer.Context(), epochs...) {
		if err != nil {
			return nil, nil, nil, errnie.Error(err)
		}

		if excursion != nil {
			if len(trainer.symbols) > 0 {
				if _, ok := trainer.symbols[excursion.Label]; !ok {
					continue
				}
			}

			byEpoch[excursion.Epoch] = append(byEpoch[excursion.Epoch], excursion)
		}
	}

	targetEpochs := sortedEpochKeys(byEpoch)
	tokensByTick := make(map[int64]string)
	allExcursions := make([]*excursionData, 0)
	tapeTicks := make([]int64, 0)
	tickOffset := int64(0)

	results := make([]epochResult, len(targetEpochs))

	for index, epoch := range targetEpochs {
		results[index] = trainer.loadEpoch(grid, epoch, byEpoch[epoch])
	}

	for _, res := range results {
		if res.err != nil {
			return nil, nil, nil, res.err
		}

		for _, localTick := range res.ticks {
			globalTick := localTick + tickOffset
			tokensByTick[globalTick] = res.tokens[localTick]
			tapeTicks = append(tapeTicks, globalTick)
		}

		for _, exc := range res.excursions {
			exc.startTick += tickOffset
			exc.bTick += tickOffset
			exc.cTick += tickOffset
			allExcursions = append(allExcursions, exc)
		}

		tickOffset += res.maxTick + 1000
	}

	slices.Sort(tapeTicks)

	slices.SortFunc(allExcursions, func(left, right *excursionData) int {
		if left.bTick < right.bTick {
			return -1
		}

		if left.bTick > right.bTick {
			return 1
		}

		return 0
	})

	return allExcursions, tokensByTick, tapeTicks, nil
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
		errnie.Warn(fmt.Sprintf(
			"[training] skipping excursion for %s (%s -> %s): %s",
			symbol, bPrice, cPrice, roundTripErr,
		))
		return nil, nil
	}

	if cost == nil || cost.Sign() <= 0 {
		return nil, nil
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

	splitIndex := max(int(float64(len(excursions))*0.70), 1)

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
		windowStart := max(record.bTick-precursorTicks, 0)

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
	excIndex := 0

	for _, tick := range evalTicks {
		token, exists := tokensByTick[tick]

		if !exists || token == "" {
			continue
		}

		if !inPosition {
			precursorTokens := sliceTokens(tokensByTick, tick-candidateTicks, tick, false)
			action := candidateTrie.Consensus(precursorTokens)

			if action == actionEnter {
				inPosition = true
				entryTick = tick

				for excIndex < len(evalExcursions) && evalExcursions[excIndex].bTick < tick {
					excIndex++
				}

				var target *excursionData

				if excIndex < len(evalExcursions) {
					target = evalExcursions[excIndex]
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

			continue
		}

		exitTokens := sliceTokens(tokensByTick, entryTick, tick, true)
		action := candidateTrie.Consensus(exitTokens)
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
	iterations := min(max(int(maxSearchTicks-minSearchTicks+1)*4, 16), 64)

	explorationConstant := math.Sqrt(2)
	memo := make(map[int64]float64)
	mctsTracker := newProgressTracker("MCTS search", int64(iterations), 3*time.Second)

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
		reward, evaluated := memo[candidateTicks]

		if !evaluated {
			candidateTrie := trainer.buildCandidateTrie(trainExcursions, tokensByTick, candidateTicks)
			outcome := trainer.evaluateReplay(candidateTrie, tokensByTick, evalTicks, evalExcursions, candidateTicks)
			reward = outcome.netPnL
			memo[candidateTicks] = reward
		}

		for node := current; node != nil; node = node.parent {
			node.visits++
			node.totalPnL += reward

			if reward > node.bestPnL {
				node.bestPnL = reward
			}
		}

		mctsTracker.Step(
			int64(iteration+1),
			fmt.Sprintf("candidateTicks=%d reward=%.4f bestPnL=%.4f", candidateTicks, reward, root.bestPnL),
		)
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
	if len(actions) > 1 {
		return
	}

	for actionKey, count := range node.actions {
		if count > 0 {
			actions[actionKey] = struct{}{}
		}
	}

	if len(actions) > 1 {
		return
	}

	for _, child := range node.children {
		collectSubtreeActions(child, actions)

		if len(actions) > 1 {
			return
		}
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
	type publishTask struct {
		key  string
		body []byte
	}

	var tasks []publishTask
	validKeys := make(map[string]struct{})

	var collectTasks func(node *precursorNode) error

	collectTasks = func(node *precursorNode) error {
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
				tasks = append(tasks, publishTask{key: blobKey, body: body})
			}
		}

		for _, child := range node.children {
			if err := collectTasks(child); err != nil {
				return err
			}
		}

		return nil
	}

	if err := collectTasks(trie.root); err != nil {
		return 0, nil, errnie.Error(err)
	}

	if len(tasks) == 0 {
		return 0, validKeys, nil
	}

	workers := min(16, len(tasks))
	totalTasks := int64(len(tasks))
	var completedCount atomic.Int64
	pubTracker := newProgressTracker("publish S3 blobs", totalTasks, 2*time.Second)
	taskChan := make(chan publishTask, len(tasks))

	for _, task := range tasks {
		taskChan <- task
	}

	close(taskChan)

	errChan := make(chan error, workers)
	var group sync.WaitGroup

	for range workers {

		group.Go(func() {

			for task := range taskChan {
				putErr := catalog.PutBlob(ctx, task.key, task.body)

				if putErr != nil && !errors.Is(putErr, tables.ErrBlobStorageUnconfigured) {
					select {
					case errChan <- errnie.Error(errnie.Err(
						errnie.IO,
						"[training] failed to put blob: "+task.key,
						putErr,
					)):
					default:
					}

					return
				}

				done := completedCount.Add(1)
				pubTracker.Step(done)
			}
		})
	}

	group.Wait()
	close(errChan)

	if err := <-errChan; err != nil {
		return 0, nil, err
	}

	return len(tasks), validKeys, nil
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
	var staleKeys []string

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
			staleKeys = append(staleKeys, existingKey)
		}
	}

	if len(staleKeys) == 0 {
		return nil
	}

	workers := min(16, len(staleKeys))
	totalStale := int64(len(staleKeys))
	var deletedCount atomic.Int64
	delTracker := newProgressTracker("cleanup stale S3 blobs", totalStale, 2*time.Second)
	keyChan := make(chan string, len(staleKeys))

	for _, key := range staleKeys {
		keyChan <- key
	}

	close(keyChan)

	errChan := make(chan error, workers)
	var group sync.WaitGroup

	for range workers {

		group.Go(func() {

			for key := range keyChan {
				deleteErr := trainer.catalog.DeleteBlob(trainer.Context(), key)

				if deleteErr != nil && !errors.Is(deleteErr, tables.ErrBlobStorageUnconfigured) {
					select {
					case errChan <- errnie.Error(errnie.Err(
						errnie.IO,
						"[training] failed to delete stale blob: "+key,
						deleteErr,
					)):
					default:
					}

					return
				}

				done := deletedCount.Add(1)
				delTracker.Step(done)
			}
		})
	}

	group.Wait()
	close(errChan)

	if err := <-errChan; err != nil {
		return err
	}

	return nil
}
