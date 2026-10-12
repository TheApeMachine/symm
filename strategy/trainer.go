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

/*
Trainer chooses the Tokenizer (precursor length, region Top-K and token
transition threshold) by Monte Carlo Tree Search over a replay of the
recorded tape, then publishes the action tree that Tokenizer produces.

The replay is prequential: it walks the tape in venue time, learns each
excursion's paths only once its outcome is known (at C), and trades every
later path that lands on a learned path with one agreed action, at the
recorded touch, paying the taker fee on both legs. Its summed net return is
the reward, so every decision it scores was made before its outcome existed
and no train/evaluation split is needed.
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

/*
pathStats is what the trainer stores in a token path's blob: means over the
excursions that produced the path.
*/
type pathStats struct {
	Gain        float64 `json:"gain"`
	HoldSeconds float64 `json:"hold_seconds"`
	Visits      int     `json:"visits"`
	MeanReward  float64 `json:"mean_reward"`
}

/*
s3KeyLimit is the S3 protocol's maximum object key length in bytes.
*/
const s3KeyLimit = 1024

/*
frame is one observed join reduced to what the replay needs: per Top-K its
token and strength exactly as Tokenizer.Token gives them, and the symbol's
last recorded touch (zero until the first quote).
*/
type frame struct {
	epoch     int64
	at        int64
	symbol    string
	tokens    [store.RegionCount]string
	strengths [store.RegionCount]float64
	bid       float64
	ask       float64
}

/*
excursion is one detected excursion: B and C in venue time, whether it is a
friction-clearing rise, and its economics.
*/
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

/*
Replay event kinds, in their order at equal time: the frames at a time move
the paths first, then B snapshots the path, then C learns from it.
*/
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

/*
recording is the replayable history: observed frames, excursions, and the events
that order them.
*/
type recording struct {
	frames     []frame
	excursions []excursion
	events     []event
	strengths  [store.RegionCount][]float64
}

/*
Generate searches the Tokenizer, then publishes the action tree it produces
excursion by excursion, the Tokenizer itself, and removes stale paths.
*/
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

/*
load reads the excursions and the sensory tape of every epoch into a tape.
*/
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

		record, err := trainer.parseExcursion(measurement)

		if err != nil {
			return nil, errnie.Error(err)
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
			epoch: history.frames[idx].epoch, at: history.frames[idx].at, kind: eventFrame, index: idx,
		})

		for regions := range store.RegionCount {
			history.strengths[regions] = append(history.strengths[regions], history.frames[idx].strengths[regions])
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

/*
parseExcursion reads one detection into an excursion. B and C are its venue
times (From and At), its prices are the exact recorded decimals.
*/
func (trainer *Trainer) parseExcursion(measurement *data.Measurement) (excursion, error) {
	bPrice, cPrice, err := tables.DetectionPrices(measurement)

	if err != nil {
		return excursion{}, errnie.Error(err)
	}

	if measurement.From.IsZero() || measurement.At.Before(measurement.From) {
		return excursion{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[trainer] excursion of %s has no ordered B and C times", measurement.Label),
			nil,
		))
	}

	pnl, cost, err := trainer.price.RoundTrip(measurement.Label, bPrice, cPrice)

	if err != nil {
		return excursion{}, errnie.Error(err)
	}

	return excursion{
		epoch:       measurement.Epoch,
		symbol:      measurement.Label,
		b:           measurement.From.UnixNano(),
		c:           measurement.At.UnixNano(),
		up:          measurement.Meta("type") == excursionUp,
		gain:        cPrice.Sub(bPrice).Div(bPrice).Float64(),
		netReturn:   pnl.Div(cost).Float64(),
		holdSeconds: measurement.At.Sub(measurement.From).Seconds(),
	}, nil
}

/*
readTape rebuilds the epoch's joins from the stored signal rows and keeps
the frames the grid observes. One ingress event is one (symbol, venue time)
group: when one Kraken message carries several trades they share the venue
time, and the k-th row of every source (in sequence order) belongs to the
k-th trade.
*/
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

		for trade := 0; ; trade++ {
			peers := make([]*data.Measurement, 0, len(bySource))

			for _, list := range bySource {
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
					quotes[key.label] = [2]float64{bid.Metric.Raw, ask.Metric.Raw}
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
		"[trainer] epoch %d: %d joins, %d observed frames so far, read in %s",
		epoch, joins, len(history.frames), time.Since(started).Round(time.Second),
	))

	return nil
}

/*
Actions a path can be learned with, as bits so a path's agreement is one
mask: it acts only when exactly one bit is set.
*/
const (
	maskEnter uint8 = 1 << iota
	maskExit
	maskNoop
)

/*
replay walks the tape under tokenizer, learning and trading as described on
Trainer, and returns the summed net return of its trades and its matches:
the frames whose completed path had already been learned. learn, when not
nil, receives every path key as it is learned with the excursion that
taught it.

A path one token deeper ends in the shallower path, so it can only match
where the shallower one matched: once a Depth has no matches, no deeper
Depth at the same Regions and Threshold can trade.
*/
func (history *recording) replay(
	price *broker.Price, tokenizer *Tokenizer, learn func(key string, record excursion),
) (float64, int, error) {
	paths := make(map[string][]string)
	pending := make(map[int][]string)
	learned := make(map[string]uint8)
	held := make(map[string]float64)
	bids := make(map[string]float64)
	epoch := history.events[0].epoch
	reward := 0.0
	matches := 0

	settle := func(symbol string, entry, exit float64) error {
		pnl, cost, err := price.RoundTrip(symbol, decimal.NewFromFloat64(entry), decimal.NewFromFloat64(exit))

		if err != nil {
			return errnie.Error(err)
		}

		reward += pnl.Div(cost).Float64()
		delete(held, symbol)
		return nil
	}

	closeAll := func() error {
		for symbol, entry := range held {
			if err := settle(symbol, entry, bids[symbol]); err != nil {
				return errnie.Error(err)
			}
		}

		return nil
	}

	for _, step := range history.events {
		if step.epoch != epoch {
			if err := closeAll(); err != nil {
				return 0, 0, errnie.Error(err)
			}

			epoch = step.epoch
			paths = make(map[string][]string)
			pending = make(map[int][]string)
		}

		switch step.kind {
		case eventFrame:
			observed := &history.frames[step.index]
			bids[observed.symbol] = observed.bid
			tokens, moved := tokenizer.Advance(
				paths[observed.symbol],
				observed.tokens[tokenizer.Regions-1],
				observed.strengths[tokenizer.Regions-1],
			)
			paths[observed.symbol] = tokens

			if !moved || len(tokens) < tokenizer.Depth {
				continue
			}

			mask, known := learned[strings.Join(tokens, "/")]

			if known {
				matches++
			}

			_, holding := held[observed.symbol]

			if mask == maskEnter && !holding && observed.ask > 0 {
				held[observed.symbol] = observed.ask
			}

			if mask == maskExit && holding {
				if err := settle(observed.symbol, held[observed.symbol], observed.bid); err != nil {
					return 0, 0, errnie.Error(err)
				}
			}
		case eventB:
			pending[step.index] = paths[history.excursions[step.index].symbol]
		case eventC:
			record := history.excursions[step.index]
			entry := pending[step.index]
			delete(pending, step.index)

			if len(entry) == tokenizer.Depth {
				key, bit, action := strings.Join(entry, "/"), maskNoop, actionNoop

				if record.up {
					bit, action = maskEnter, actionEnter
				}

				learned[key] |= bit

				if learn != nil {
					learn(key+"/"+action, record)
				}
			}

			exit := paths[record.symbol]

			if record.up && len(exit) == tokenizer.Depth {
				key := strings.Join(exit, "/")
				learned[key] |= maskExit

				if learn != nil {
					learn(key+"/"+actionExit, record)
				}
			}
		}
	}

	if err := closeAll(); err != nil {
		return 0, 0, errnie.Error(err)
	}

	return reward, matches, nil
}

/*
searchNode is a box of Tokenizer parameters for one Top-K: depths
[depthLo, depthHi] and threshold ranks [rankLo, rankHi], where rank r is the
r-th smallest token strength the tape produced for that Top-K. Its candidate
is the box's midpoint.
*/
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

/*
expand halves the box along the dimension that needs more halvings to reach
its statistical resolution limit, depth on a tie.
*/
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

/*
choose returns the first unvisited child, otherwise the child with the
highest UCB1 score, its best reward rescaled into [0, 1] above zero hurdle.
*/
func (node *searchNode) choose(highest float64) *searchNode {
	var best *searchNode
	bestScore := math.Inf(-1)

	for _, child := range node.children {
		if child.visits == 0 {
			return child
		}

		exploit := 0.0

		if highest > 0 && child.best > 0 {
			exploit = child.best / highest
		}

		score := exploit + math.Sqrt(2*math.Log(float64(node.visits))/float64(child.visits))

		if score > bestScore {
			best, bestScore = child, score
		}
	}

	return best
}

/*
search runs MCTS over the Tokenizer parameters. The root holds one box per
Top-K; each box spans every depth whose longest key fits S3's key limit and
every threshold rank. Every iteration descends by UCB1, scores the first
unvisited box's candidate by replay, and backs the reward up. The search
ends when the descent arrives at an already scored box at the statistical
resolution limit of the data: the tree has converged on it.
*/
func (trainer *Trainer) search(history *recording) (*Tokenizer, error) {
	root := &searchNode{}

	for regions := 1; regions <= store.RegionCount; regions++ {
		// A key is Depth tokens of Regions "Rxx" names joined by "-",
		// the tokens joined by "/", then "/" and the action: 4·Regions·Depth
		// bytes plus the longest action name.
		root.children = append(root.children, &searchNode{
			regions: regions,
			depthLo: 1,
			depthHi: (s3KeyLimit - len(actionEnter)) / (4 * regions),
			rankLo:  0,
			rankHi:  len(history.frames) - 1,
			best:    math.Inf(-1),
		})
	}

	scores := make(map[Tokenizer]float64)
	highest := math.Inf(-1)
	var best *Tokenizer
	started := time.Now()

	for iteration := 1; ; iteration++ {
		node := root
		trail := []*searchNode{root}

		for {
			node = node.choose(highest)
			trail = append(trail, node)

			if node.visits == 0 {
				break
			}

			if node.terminal(history) {
				if best != nil && node.best == highest {
					errnie.Info(fmt.Sprintf(
						"[trainer] search converged on best candidate after %d iterations (%d replays, %s): depth=%d regions=%d threshold=%g reward=%.6f",
						iteration, len(scores), time.Since(started).Round(time.Second),
						best.Depth, best.Regions, best.Threshold, highest,
					))

					return best, nil
				}

				break
			}

			if node.children == nil {
				node.expand(history)
			}
		}

		tokenizer := node.candidate(history)
		reward, scored := scores[*tokenizer]

		if !scored {
			reward, matches, err := history.replay(trainer.price, tokenizer, nil)

			if err != nil {
				return nil, errnie.Error(err)
			}

			scores[*tokenizer] = reward

			if matches == 0 && tokenizer.Depth <= node.depthHi {
				node.depthHi = max(node.depthLo, tokenizer.Depth-1)
			}

			if reward > highest {
				highest = reward
				best = tokenizer

				errnie.Info(fmt.Sprintf(
					"[trainer] iteration %d: best so far depth=%d regions=%d threshold=%g reward=%.6f",
					iteration, tokenizer.Depth, tokenizer.Regions, tokenizer.Threshold, reward,
				))
			}
		}

		highest = max(highest, reward)

		for _, visited := range trail {
			visited.visits++
			visited.best = max(visited.best, reward)
		}
	}
}

/*
publish replays the tape once more under the chosen Tokenizer, writes every
learned path with its statistics in the order the excursions taught them,
then the Tokenizer, and finally deletes the paths no longer learned.
*/
func (trainer *Trainer) publish(history *recording, tokenizer *Tokenizer) error {
	type totals struct {
		visits int
		gain   float64
		hold   float64
		reward float64
	}

	stats := make(map[string]*totals)
	order := make([]string, 0)

	reward, _, err := history.replay(trainer.price, tokenizer, func(key string, record excursion) {
		entry, found := stats[key]

		if !found {
			entry = &totals{}
			stats[key] = entry
			order = append(order, key)
		}

		entry.visits++
		entry.gain += record.gain
		entry.hold += record.holdSeconds
		entry.reward += record.netReturn
	})

	if err != nil {
		return errnie.Error(err)
	}

	valid := make(map[string]struct{}, len(order))
	counts := make(map[string]int)

	for _, key := range order {
		entry := stats[key]
		visits := float64(entry.visits)

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
		counts[key[strings.LastIndex(key, "/")+1:]]++
	}

	body, err := json.Marshal(tokenizer)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[trainer] unable to encode the tokenizer", err))
	}

	if err := trainer.catalog.PutBlob(trainer.Context(), tokenizerKey, body); err != nil {
		return errnie.Error(err)
	}

	if err := trainer.cleanupStaleBlobs(valid); err != nil {
		return errnie.Error(err)
	}

	errnie.Info(fmt.Sprintf(
		"[trainer] published %d paths (enter=%d exit=%d noop=%d) and %s; replay net return %.6f",
		len(order), counts[actionEnter], counts[actionExit], counts[actionNoop], tokenizerKey, reward,
	))

	return nil
}

/*
cleanupStaleBlobs deletes every stored path key the chosen Tokenizer did not
learn.
*/
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

	errnie.Info(fmt.Sprintf("[trainer] deleting %d stale paths", len(stale)))

	for _, key := range stale {
		if err := trainer.catalog.DeleteBlob(trainer.Context(), key); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}
