package strategy

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"math/bits"
	"slices"
	"strings"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
)

const (
	s3KeyLimit          = 1024
	maxPrecursorDepth   = 8
	maxSearchIterations = 350
	maxSearchDuration   = 25 * time.Second
	minTradesHurdle     = 1
	inactionPenalty     = -1.0
)

const (
	maskEnter uint8 = 1 << iota
	maskExit
	maskNoop
)

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
	return &Trainer{
		System:  runtime.NewSystem(ctx, "trainer"),
		catalog: catalog,
		price:   price,
	}
}

func (trainer *Trainer) SetSymbols(symbols ...string) {
	trainer.symbols = make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		trainer.symbols[symbol] = struct{}{}
	}
}

type pathStats struct {
	Gain        float64 `json:"gain"`
	HoldSeconds float64 `json:"hold_seconds"`
	Visits      int     `json:"visits"`
	MeanReward  float64 `json:"mean_reward"`
}

type frame struct {
	epoch     int64
	at        int64
	symbol    string
	tokens    [store.RegionCount]string
	strengths [store.RegionCount]float64
	bid       float64
	ask       float64
}

type excursion struct {
	epoch       int64
	symbol      string
	b           int64
	c           int64
	up          bool
	gain        float64
	netReturn   float64
	holdSeconds float64
}

const (
	eventFrame = iota
	eventB
	eventC
)

type event struct {
	epoch int64
	at    int64
	kind  int
	index int
}

type recording struct {
	frames     []frame
	excursions []excursion
	events     []event
	strengths  [store.RegionCount][]float64
}

func (trainer *Trainer) Generate(epochs ...int64) error {
	if trainer.price == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[trainer] price system is required to evaluate excursions", nil,
		))
	}

	history, err := trainer.load(epochs...)
	if err != nil {
		return errnie.Error(err)
	}

	tokenizer, err := trainer.search(history)
	if err != nil {
		return errnie.Error(err)
	}

	return errnie.Error(trainer.publish(history, tokenizer))
}

func (trainer *Trainer) load(epochs ...int64) (*recording, error) {
	history := &recording{}
	epochSet := make(map[int64]struct{})

	for measurement, err := range trainer.catalog.Excursions(trainer.Context(), epochs...) {
		if err != nil {
			return nil, errnie.Error(err)
		}

		if !trainer.admits(measurement.Label) {
			continue
		}

		class := measurement.Meta("type")

		if class != excursionUp && class != excursionDown {
			continue
		}

		record, err := trainer.parseExcursion(measurement)
		if err != nil {
			continue
		}

		history.excursions = append(history.excursions, record)
		epochSet[record.epoch] = struct{}{}
	}

	if len(history.excursions) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "[trainer] no excursions found", nil,
		))
	}

	for _, epoch := range slices.Sorted(maps.Keys(epochSet)) {
		if err := trainer.readTape(history, epoch); err != nil {
			return nil, errnie.Error(err)
		}
	}

	if len(history.frames) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "[trainer] the tape produced no observed frames", nil,
		))
	}

	for idx := range history.frames {
		history.events = append(history.events, event{
			epoch: history.frames[idx].epoch,
			at:    history.frames[idx].at,
			kind:  eventFrame,
			index: idx,
		})

		for regions := range store.RegionCount {
			history.strengths[regions] = append(
				history.strengths[regions],
				history.frames[idx].strengths[regions],
			)
		}
	}

	for regions := range store.RegionCount {
		slices.Sort(history.strengths[regions])
	}

	for idx, record := range history.excursions {
		history.events = append(history.events,
			event{epoch: record.epoch, at: record.b, kind: eventB, index: idx},
			event{epoch: record.epoch, at: record.c, kind: eventC, index: idx},
		)
	}

	slices.SortFunc(history.events, func(left, right event) int {
		return cmp.Or(
			cmp.Compare(left.epoch, right.epoch),
			cmp.Compare(left.at, right.at),
			cmp.Compare(left.kind, right.kind),
			cmp.Compare(left.index, right.index),
		)
	})

	errnie.Info(fmt.Sprintf(
		"[trainer] tape loaded: %d observed frames, %d excursions",
		len(history.frames), len(history.excursions),
	))

	return history, nil
}

func (trainer *Trainer) admits(symbol string) bool {
	if len(trainer.symbols) == 0 {
		return true
	}
	_, admitted := trainer.symbols[symbol]
	return admitted
}

func (trainer *Trainer) parseExcursion(measurement *data.Measurement) (excursion, error) {
	class := measurement.Meta("type")

	if class != excursionUp && class != excursionDown {
		return excursion{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[trainer] excursion %s has unsupported class %q", measurement.Label, class),
			nil,
		))
	}

	bPrice, cPrice, err := tables.DetectionPrices(measurement)
	if err != nil {
		return excursion{}, errnie.Error(err)
	}

	if measurement.From.IsZero() || !measurement.At.After(measurement.From) {
		return excursion{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[trainer] excursion %s has invalid/zero duration", measurement.Label),
			nil,
		))
	}

	if bPrice.Sign() == 0 {
		return excursion{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[trainer] excursion %s has zero entry price", measurement.Label),
			nil,
		))
	}

	if bPrice.Cmp(cPrice) == 0 {
		return excursion{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[trainer] excursion %s has zero price movement (flat tape)", measurement.Label),
			nil,
		))
	}

	pnl, cost, err := trainer.price.RoundTrip(measurement.Label, bPrice, cPrice)
	if err != nil {
		return excursion{}, errnie.Error(err)
	}

	if cost.Sign() == 0 {
		return excursion{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[trainer] excursion %s has zero cost", measurement.Label),
			nil,
		))
	}

	gain := cPrice.Sub(bPrice).Div(bPrice).Float64()
	if math.Abs(gain) <= 1e-6 {
		return excursion{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[trainer] excursion %s is flat tape (gain %g)", measurement.Label, gain),
			nil,
		))
	}

	return excursion{
		epoch:       measurement.Epoch,
		symbol:      measurement.Label,
		b:           measurement.From.UnixNano(),
		c:           measurement.At.UnixNano(),
		up:          class == excursionUp,
		gain:        gain,
		netReturn:   pnl.Div(cost).Float64(),
		holdSeconds: measurement.At.Sub(measurement.From).Seconds(),
	}, nil
}

func (trainer *Trainer) readTape(history *recording, epoch int64) error {
	type groupKey struct {
		label string
		at    int64
	}

	started := time.Now()
	groups := make(map[groupKey][]*data.Measurement)

	for measurement, err := range trainer.catalog.SignalLogic(
		trainer.Context(), epoch, "", 0, 0, tables.SensorySources...,
	) {
		if err != nil {
			return errnie.Error(err)
		}
		if !trainer.admits(measurement.Label) {
			continue
		}

		key := groupKey{measurement.Label, measurement.At.UnixNano()}
		groups[key] = append(groups[key], measurement)
	}

	keys := make([]groupKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}

	slices.SortFunc(keys, func(left, right groupKey) int {
		return cmp.Or(cmp.Compare(left.at, right.at), strings.Compare(left.label, right.label))
	})

	grid := store.NewGrid(trainer.Context())
	quotes := make(map[string][2]float64)
	joins := 0

	for _, key := range keys {
		rows := groups[key]
		slices.SortFunc(rows, func(left, right *data.Measurement) int {
			return cmp.Compare(left.SeqIdx, right.SeqIdx)
		})

		bySource := make(map[string][]*data.Measurement)
		for _, row := range rows {
			bySource[row.Source] = append(bySource[row.Source], row)
		}

		// Sort source keys to guarantee 100% deterministic peer ordering
		sourceKeys := slices.Sorted(maps.Keys(bySource))

		for trade := 0; ; trade++ {
			peers := make([]*data.Measurement, 0, len(bySource))
			for _, src := range sourceKeys {
				list := bySource[src]
				if trade < len(list) {
					peers = append(peers, list[trade])
				}
			}

			if len(peers) == 0 {
				break
			}

			joins++
			join := data.NewMeasurement(epoch, key.label, "runtime:join", 0, peers[0].Tick)
			join.Peers(peers...)
			join.At = time.Unix(0, key.at).UTC()
			join.From = join.At
			join = join.Write()

			for _, peer := range peers {
				if peer.Source != "liquidity" {
					continue
				}
				bid := data.Pull(peer.Read("best_bid_price"))
				ask := data.Pull(peer.Read("best_ask_price"))
				if bid != nil && ask != nil {
					quotes[key.label] = [2]float64{bid.Metric.Raw(), ask.Metric.Raw()}
				}
			}

			regions := grid.Observe(join)
			if regions == nil {
				continue
			}

			observed := frame{
				epoch:  epoch,
				at:     key.at,
				symbol: key.label,
				bid:    quotes[key.label][0],
				ask:    quotes[key.label][1],
			}

			for idx := range store.RegionCount {
				observed.tokens[idx], observed.strengths[idx] = (&Tokenizer{Regions: idx + 1}).Token(regions)
			}

			history.frames = append(history.frames, observed)
		}

		delete(groups, key)
	}

	if err := grid.Error(); err != nil {
		return errnie.Error(err)
	}

	errnie.Info(fmt.Sprintf(
		"[trainer] epoch %d: %d joins, %d observed frames, read in %s",
		epoch, joins, len(history.frames), time.Since(started).Round(time.Second),
	))

	return nil
}

/*
replay conducts a two-pass evaluation:
Pass 1: Identifies prefix outcomes across the epoch. Contaminated prefixes
(observed leading to BOTH rises and non-rises) are stripped of entry rights.
Pass 2: Simulates prequential trading. Only uncontradicted, pure prefixes
trade once discovered at C, ensuring non-executable actions never earn rewards.
*/
func (history *recording) replay(
	price *broker.Price, tokenizer *Tokenizer, learn func(path string, record excursion),
) (float64, int, map[string]uint8, error) {
	if len(history.events) == 0 {
		return inactionPenalty, 0, nil, nil
	}

	// -------------------------------------------------------------
	// PASS 1: Identify Prefix Purity Across the Entire Epoch
	// -------------------------------------------------------------
	type pathTally struct {
		ups   int
		downs int
		exits int
	}

	tallies := make(map[string]*pathTally)
	p1Paths := make(map[string][]string)
	p1Pending := make(map[int][]string)
	p1Epoch := history.events[0].epoch

	getTally := func(k string) *pathTally {
		t, exists := tallies[k]
		if !exists {
			t = &pathTally{}
			tallies[k] = t
		}
		return t
	}

	for _, step := range history.events {
		if step.epoch != p1Epoch {
			p1Epoch = step.epoch
			p1Paths = make(map[string][]string)
			p1Pending = make(map[int][]string)
		}

		switch step.kind {
		case eventFrame:
			observed := &history.frames[step.index]
			tokens, moved := tokenizer.Advance(
				p1Paths[observed.symbol],
				observed.tokens[tokenizer.Regions-1],
				observed.strengths[tokenizer.Regions-1],
			)
			if moved {
				p1Paths[observed.symbol] = tokens
			}

		case eventB:
			p1Pending[step.index] = slices.Clone(p1Paths[history.excursions[step.index].symbol])

		case eventC:
			record := history.excursions[step.index]
			entry := p1Pending[step.index]
			delete(p1Pending, step.index)

			if len(entry) == tokenizer.Depth {
				key := strings.Join(entry, "/")
				if record.up && record.gain > 0 {
					getTally(key).ups++
				}

				if !record.up && record.gain < 0 {
					getTally(key).downs++
				}
			}

			exit := p1Paths[record.symbol]
			if record.up && len(exit) == tokenizer.Depth {
				getTally(strings.Join(exit, "/")).exits++
			}
		}
	}

	// Determine globally unambiguous, executable policies
	executable := make(map[string]uint8)
	for key, tally := range tallies {
		if tally.ups > 0 && tally.downs == 0 && tally.exits == 0 {
			executable[key] = maskEnter
		}

		if tally.exits > 0 && tally.ups == 0 {
			executable[key] = maskExit
		}

		if tally.downs > 0 && tally.ups == 0 && tally.exits == 0 {
			executable[key] = maskNoop
		}
	}

	// -------------------------------------------------------------
	// PASS 2: Simulate Venue-Time Execution with Pure Policies Only
	// -------------------------------------------------------------
	paths := make(map[string][]string)
	pending := make(map[int][]string)
	discovered := make(map[string]uint8)
	held := make(map[string]float64)
	bids := make(map[string]float64)
	epoch := history.events[0].epoch
	reward := 0.0
	matches := 0
	trades := 0

	settle := func(symbol string, entry, exit float64) error {
		pnl, cost, err := price.RoundTrip(
			symbol,
			decimal.NewFromFloat64(entry),
			decimal.NewFromFloat64(exit),
		)
		if err != nil {
			return errnie.Error(err)
		}
		reward += pnl.Div(cost).Float64()
		delete(held, symbol)
		return nil
	}

	closeAll := func() error {
		for symbol, entry := range held {
			bid := bids[symbol]
			if bid <= 0 {
				bid = entry // Prevent artificial 100% loss on quote dropout
			}
			if err := settle(symbol, entry, bid); err != nil {
				return errnie.Error(err)
			}
		}
		return nil
	}

	for _, step := range history.events {
		if step.epoch != epoch {
			if err := closeAll(); err != nil {
				return inactionPenalty, 0, nil, errnie.Error(err)
			}
			epoch = step.epoch
			paths = make(map[string][]string)
			pending = make(map[int][]string)
		}

		switch step.kind {
		case eventFrame:
			observed := &history.frames[step.index]
			if observed.bid > 0 {
				bids[observed.symbol] = observed.bid
			}

			tokens, moved := tokenizer.Advance(
				paths[observed.symbol],
				observed.tokens[tokenizer.Regions-1],
				observed.strengths[tokenizer.Regions-1],
			)
			if moved {
				paths[observed.symbol] = tokens
			}

			if !moved || len(tokens) < tokenizer.Depth {
				continue
			}

			pathKey := strings.Join(tokens, "/")
			mask, isDiscovered := discovered[pathKey]

			if isDiscovered {
				matches++
			}

			_, holding := held[observed.symbol]

			// Only trade if prefix is already discovered at C AND strictly pure
			if mask == maskEnter && !holding && observed.ask > 0 {
				held[observed.symbol] = observed.ask
				trades++
			}

			if (mask&maskExit != 0) && holding && observed.bid > 0 {
				if err := settle(observed.symbol, held[observed.symbol], observed.bid); err != nil {
					return inactionPenalty, 0, nil, errnie.Error(err)
				}
			}

		case eventB:
			pending[step.index] = slices.Clone(paths[history.excursions[step.index].symbol])

		case eventC:
			record := history.excursions[step.index]
			entry := pending[step.index]
			delete(pending, step.index)

			if len(entry) == tokenizer.Depth {
				key := strings.Join(entry, "/")
				if pureMask, isPure := executable[key]; isPure {
					discovered[key] |= pureMask
					if learn != nil {
						learn(key, record)
					}
				}
			}

			exit := paths[record.symbol]
			if record.up && len(exit) == tokenizer.Depth {
				key := strings.Join(exit, "/")
				if pureMask, isPure := executable[key]; isPure && pureMask == maskExit {
					discovered[key] |= maskExit
					if learn != nil {
						learn(key, record)
					}
				}
			}
		}
	}

	if err := closeAll(); err != nil {
		return inactionPenalty, 0, nil, errnie.Error(err)
	}

	// Penalize inactivity so non-trading never beats an active policy
	if trades < minTradesHurdle {
		return inactionPenalty, matches, executable, nil
	}

	return reward, matches, executable, nil
}

type searchNode struct {
	regions  int
	depthLo  int
	depthHi  int
	rankLo   int
	rankHi   int
	visits   int
	best     float64
	children []*searchNode
}

func (node *searchNode) candidate(history *recording) *Tokenizer {
	return &Tokenizer{
		Depth:     (node.depthLo + node.depthHi) / 2,
		Regions:   node.regions,
		Threshold: history.strengths[node.regions-1][(node.rankLo+node.rankHi)/2],
	}
}

func (node *searchNode) terminal(history *recording) bool {
	if node.depthLo < node.depthHi {
		return false
	}
	mid := (node.rankLo + node.rankHi) / 2
	count := len(history.frames)
	uncertainty := max(1, int(math.Sqrt(float64(mid)*float64(count-mid)/float64(count))))
	return node.rankHi-node.rankLo <= uncertainty
}

func (node *searchNode) expand(history *recording) {
	depthSplits := bits.Len(uint(node.depthHi - node.depthLo))
	mid := (node.rankLo + node.rankHi) / 2
	count := len(history.frames)
	uncertainty := max(1, int(math.Sqrt(float64(mid)*float64(count-mid)/float64(count))))
	rankSplits := 0

	if node.rankHi-node.rankLo > uncertainty {
		rankSplits = bits.Len(uint((node.rankHi - node.rankLo) / uncertainty))
	}

	low, high := *node, *node
	low.visits, low.best, low.children = 0, math.Inf(-1), nil
	high.visits, high.best, high.children = 0, math.Inf(-1), nil

	if depthSplits >= rankSplits && depthSplits > 0 {
		low.depthHi = (node.depthLo + node.depthHi) / 2
		high.depthLo = low.depthHi + 1
	}

	if rankSplits > depthSplits && rankSplits > 0 {
		low.rankHi = (node.rankLo + node.rankHi) / 2
		high.rankLo = low.rankHi + 1
	}

	node.children = []*searchNode{&low, &high}
}

func (node *searchNode) choose(minReward, maxReward float64) *searchNode {
	var best *searchNode
	bestScore := math.Inf(-1)

	for _, child := range node.children {
		if child.visits == 0 {
			return child
		}

		exploit := 0.0
		if maxReward > minReward && !math.IsInf(child.best, -1) {
			exploit = (child.best - minReward) / (maxReward - minReward)
		}

		score := exploit + math.Sqrt(2*math.Log(float64(node.visits))/float64(child.visits))
		if score > bestScore {
			best, bestScore = child, score
		}
	}

	return best
}

func (trainer *Trainer) search(history *recording) (*Tokenizer, error) {
	root := &searchNode{best: math.Inf(-1)}

	for regions := 1; regions <= store.RegionCount; regions++ {
		maxDepth := (s3KeyLimit - len(actionEnter)) / (4 * regions)
		maxDepth = min(maxPrecursorDepth, max(1, maxDepth))

		root.children = append(root.children, &searchNode{
			regions: regions,
			depthLo: 1,
			depthHi: maxDepth,
			rankLo:  0,
			rankHi:  len(history.frames) - 1,
			best:    math.Inf(-1),
		})
	}

	scores := make(map[Tokenizer]float64)
	highest := math.Inf(-1)
	minReward := math.Inf(1)
	maxReward := math.Inf(-1)
	var best *Tokenizer
	started := time.Now()

	for iteration := 1; iteration <= maxSearchIterations; iteration++ {
		select {
		case <-trainer.Context().Done():
			if best != nil {
				return best, nil
			}
			return nil, trainer.Context().Err()
		default:
		}

		if time.Since(started) >= maxSearchDuration && best != nil {
			break
		}

		node := root
		trail := []*searchNode{root}

		for {
			node = node.choose(minReward, maxReward)
			trail = append(trail, node)

			if node.visits == 0 {
				break
			}

			if node.terminal(history) {
				break
			}

			if node.children == nil {
				node.expand(history)
			}
		}

		tokenizer := node.candidate(history)
		reward, scored := scores[*tokenizer]

		if !scored {
			var err error
			reward, _, _, err = history.replay(trainer.price, tokenizer, nil)
			if err != nil {
				return nil, errnie.Error(err)
			}

			scores[*tokenizer] = reward
			minReward = min(minReward, reward)
			maxReward = max(maxReward, reward)

			if reward > highest {
				highest = reward
				best = tokenizer

				errnie.Info(fmt.Sprintf(
					"[trainer] iter %d: new best depth=%d regions=%d threshold=%.5f reward=%.6f",
					iteration, tokenizer.Depth, tokenizer.Regions, tokenizer.Threshold, reward,
				))
			}
		}

		for _, visited := range trail {
			visited.visits++
			visited.best = max(visited.best, reward)
		}
	}

	if best == nil {
		best = root.children[0].candidate(history)
	}

	errnie.Info(fmt.Sprintf(
		"[trainer] search complete (%d evaluated, %s): depth=%d regions=%d threshold=%.5f reward=%.6f",
		len(scores), time.Since(started).Round(time.Second),
		best.Depth, best.Regions, best.Threshold, highest,
	))

	return best, nil
}

func (trainer *Trainer) publish(history *recording, tokenizer *Tokenizer) error {
	type totals struct {
		visits int
		gain   float64
		hold   float64
		reward float64
	}

	stats := make(map[string]*totals)
	order := make([]string, 0)

	reward, _, learned, err := history.replay(trainer.price, tokenizer, func(path string, record excursion) {
		entry, found := stats[path]
		if !found {
			entry = &totals{}
			stats[path] = entry
			order = append(order, path)
		}

		entry.visits++
		entry.gain += record.gain
		entry.hold += record.holdSeconds
		entry.reward += record.netReturn
	})
	if err != nil {
		return errnie.Error(err)
	}

	valid := make(map[string]struct{})
	counts := make(map[string]int)

	for _, path := range order {
		entry := stats[path]
		if entry.visits == 0 {
			continue
		}
		visits := float64(entry.visits)

		// Prefixes are pure; unanimous actionable and noop friction paths are published
		var action string
		if learned[path] == maskEnter {
			action = actionEnter
		}

		if learned[path] == maskExit {
			action = actionExit
		}

		if learned[path] == maskNoop {
			action = actionNoop
		}

		if action == "" {
			continue
		}

		key := path + "/" + action

		body, err := json.Marshal(pathStats{
			Gain:        entry.gain / visits,
			HoldSeconds: entry.hold / visits,
			Visits:      entry.visits,
			MeanReward:  entry.reward / visits,
		})
		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "[trainer] unable to encode stats of "+key, err))
		}

		if err := trainer.catalog.PutBlob(trainer.Context(), key, body); err != nil {
			return errnie.Error(err)
		}

		valid[key] = struct{}{}
		counts[action]++
	}

	body, err := json.Marshal(tokenizer)
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[trainer] unable to encode tokenizer", err))
	}

	if err := trainer.catalog.PutBlob(trainer.Context(), tokenizerKey, body); err != nil {
		return errnie.Error(err)
	}

	if err := trainer.cleanupStaleBlobs(valid); err != nil {
		return errnie.Error(err)
	}

	errnie.Info(fmt.Sprintf(
		"[trainer] published %d unanimous paths (enter=%d exit=%d noop=%d); replay net return: %.6f",
		len(valid), counts[actionEnter], counts[actionExit], counts[actionNoop], reward,
	))

	return nil
}

func (trainer *Trainer) cleanupStaleBlobs(valid map[string]struct{}) error {
	stale := make([]string, 0)

	for key, err := range trainer.catalog.ListBlobs(trainer.Context(), "") {
		if err != nil {
			return errnie.Error(err)
		}

		if !strings.HasSuffix(key, "/"+actionEnter) &&
			!strings.HasSuffix(key, "/"+actionExit) &&
			!strings.HasSuffix(key, "/"+actionNoop) {
			continue
		}

		if _, found := valid[key]; !found {
			stale = append(stale, key)
		}
	}

	for _, key := range stale {
		if err := trainer.catalog.DeleteBlob(trainer.Context(), key); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}
