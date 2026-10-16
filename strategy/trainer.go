package strategy

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

/*
Policies are published under data.data.PolicyPrefixes(epoch):
one Value per learned path, pooled across the epoch's symbols.
*/
/*
Trainer learns which grammar paths of the signal grid precede profitable
excursions. Its inputs are the stored ticks of an epoch (one full tick Value
per key) and the excursions detect wrote for that epoch.
*/
type Trainer struct {
	*runtime.System
	client  *store.Client
	price   *broker.Price
	symbols []string
}

func NewTrainer(
	ctx context.Context,
	client *store.Client,
	price *broker.Price,
	symbols []string,
) *Trainer {
	return &Trainer{
		System:  runtime.NewSystem(ctx, "trainer"),
		client:  client,
		price:   price,
		symbols: symbols,
	}
}

type pathStats struct {
	gain        float64
	holdSeconds float64
	visits      int
}

type pathTally struct {
	ups   int
	downs int
	exits int
}

type frameRecord struct {
	tick uint64
	path string
}

/*
symbolTape is the grammar state of one symbol's grid frames over an epoch.
*/
type symbolTape struct {
	grammar      *Grammar
	lastSym      string
	recentTokens []string
	frames       []frameRecord
}

func newSymbolTape() *symbolTape {
	return &symbolTape{grammar: NewGrammar(), recentTokens: make([]string, 0, 16)}
}

/*
tapeStep is what one grid frame did to a symbol's grammar: the active regions
and the midpoint gate over them, the region tokens that fired, whether that
chord merely repeated the previous one (nothing is appended then), the rule
the append completed, and the grammar path active after it. The trainer
tallies excursions against Path; live inspection shows every field.
*/
type tapeStep struct {
	Active   []*nmstore.Region
	Midpoint float64
	Fired    []string
	Repeat   bool
	Rule     *Rule
	Path     string
}

/*
advance turns one frame's regions into region tokens and appends them to the
grammar. It reports false when no region is active, so the frame has no chord.
*/
func (tape *symbolTape) advance(regions []*nmstore.Region) (tapeStep, bool) {
	step := tapeStep{Active: make([]*nmstore.Region, 0)}

	for _, region := range regions {
		if region.Brightness > 0 {
			step.Active = append(step.Active, region)
		}
	}

	if len(step.Active) == 0 {
		return step, false
	}

	// Dynamic midpoint activation gate.
	step.Midpoint = (step.Active[0].Brightness + step.Active[len(step.Active)-1].Brightness) / 2.0
	step.Fired = make([]string, 0)

	for _, region := range step.Active {
		if region.Brightness >= step.Midpoint {
			step.Fired = append(step.Fired, fmt.Sprintf("R%02d", region.Key))
		}
	}

	chordKey := strings.Join(step.Fired, "/")

	if chordKey == tape.lastSym {
		step.Repeat = true
		return step, true
	}

	tape.lastSym = chordKey

	for _, token := range step.Fired {
		if rule := tape.grammar.Append(token); rule != nil {
			step.Rule = rule
		}
	}

	tape.recentTokens = append(tape.recentTokens, step.Fired...)

	if len(tape.recentTokens) > 16 {
		tape.recentTokens = tape.recentTokens[len(tape.recentTokens)-16:]
	}

	if step.Rule != nil {
		if phrase := step.Rule.TerminalNames(tape.grammar); len(phrase) >= 2 {
			step.Path = strings.Join(phrase, "/")
		}
	}

	if step.Path == "" && len(tape.recentTokens) >= 2 {
		step.Path = strings.Join(tape.recentTokens[len(tape.recentTokens)-2:], "/")
	} else if step.Path == "" && len(tape.recentTokens) == 1 {
		step.Path = tape.recentTokens[0]
	}

	return step, true
}

/*
observe records the grammar path active at tick for one frame's regions.
*/
func (tape *symbolTape) observe(tick uint64, regions []*nmstore.Region) {
	step, ok := tape.advance(regions)

	if !ok {
		return
	}

	tape.frames = append(tape.frames, frameRecord{tick: tick, path: step.Path})
}

/*
frameAt is the index of the last frame at or before tick, or -1 when the
tape has no frame that early.
*/
func (tape *symbolTape) frameAt(tick uint64) int {
	idx, found := slices.BinarySearchFunc(tape.frames, tick, func(frame frameRecord, target uint64) int {
		switch {
		case frame.tick < target:
			return -1
		case frame.tick > target:
			return 1
		}
		return 0
	})

	if found {
		for idx+1 < len(tape.frames) && tape.frames[idx+1].tick == tick {
			idx++
		}

		return idx
	}

	return idx - 1
}

func (tape *symbolTape) findPath(idx, minIdx int) string {
	for i := idx; i >= minIdx && i >= 0; i-- {
		if tape.frames[i].path != "" {
			return tape.frames[i].path
		}
	}

	return ""
}

/*
Generate trains on each epoch and publishes its policies.
*/
func (trainer *Trainer) Generate(epochs ...uint64) error {
	if trainer.client == nil || trainer.price == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "[trainer] trainer requires client and price", nil))
	}

	horizon := system.Cfg.Learning.PrecursorHorizon

	// learning.precursor_horizon has its default in system; a non-positive
	// value is a broken config.
	if horizon <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[trainer] learning.precursor_horizon %d is not positive", horizon),
			nil,
		))
	}

	for _, epoch := range epochs {
		if err := trainer.generate(epoch, horizon); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

func (trainer *Trainer) generate(epoch uint64, horizon int) error {
	grid := nmstore.NewGrid(trainer.Context())
	tapes := make(map[string]*symbolTape)
	order := make([]string, 0)
	ticks := 0

	for value := range store.Values(trainer.client, data.TickPrefix(epoch)) {
		if err := trainer.Context().Err(); err != nil {
			return err
		}

		symbolTicks, err := broker.ReadTick(value)

		if err != nil {
			return err
		}

		ticks++

		for _, symbolTick := range symbolTicks {
			symbol := symbolTick.Symbol

			if len(trainer.symbols) > 0 && !slices.Contains(trainer.symbols, symbol) {
				continue
			}

			tape, ok := tapes[symbol]

			if !ok {
				tape = newSymbolTape()
				tapes[symbol] = tape
				order = append(order, symbol)
			}

			frame, err := grid.Observe(value, symbol)

			if err != nil {
				return err
			}

			if len(frame.Regions) > 0 {
				tape.observe(value.Tick, frame.Regions)
			}
		}
	}

	if ticks == 0 {
		return errnie.Err(errnie.NotFound, "[trainer] no stored ticks under "+data.TickPrefix(epoch), nil)
	}

	tallies := make(map[string]*pathTally)
	stats := make(map[string]*pathStats)
	excursions, unplaced := 0, 0

	for _, symbol := range order {
		tape := tapes[symbol]

		for excursion := range store.Values(trainer.client, data.ExcursionPrefixes(symbol, epoch)...) {
			if excursion.Role != "up" && excursion.Role != "down" {
				continue
			}

			excursions++

			placed, err := trainer.tally(symbol, tape, excursion, horizon, tallies, stats)

			if err != nil {
				return err
			}

			if !placed {
				unplaced++
			}
		}
	}

	if excursions == 0 {
		return errnie.Err(errnie.NotFound, fmt.Sprintf(
			"[trainer] epoch %d has no up/down excursions for %d symbols: run detect on it first", epoch, len(order),
		), nil)
	}

	errnie.Info(fmt.Sprintf(
		"[trainer] epoch %d: %d ticks, %d symbols, %d up/down excursions (%d with no grid path before B)",
		epoch, ticks, len(order), excursions, unplaced,
	))

	return trainer.publishPolicies(epoch, tallies, stats)
}

/*
tally attributes one excursion to the grammar paths active before its
ignition B and at its exhaustion C. The long round trip buys at the ask the
tick recorded at B and sells at the bid recorded at C. It reports false when
the symbol's tape has no grid path before B.
*/
func (trainer *Trainer) tally(
	symbol string,
	tape *symbolTape,
	excursion *data.Value,
	horizon int,
	tallies map[string]*pathTally,
	stats map[string]*pathStats,
) (bool, error) {
	fields, err := data.ExcursionFields(excursion)

	if err != nil {
		return false, err
	}

	pnl, cost, err := trainer.price.RoundTrip(
		symbol, decimal.NewFromFloat64(fields["b_ask"]), decimal.NewFromFloat64(fields["c_bid"]),
	)

	if err != nil {
		return false, errnie.Err(errnie.Validation, "[trainer] round trip of "+excursion.Prefix, err)
	}

	if cost.Sign() <= 0 {
		return false, errnie.Err(errnie.Validation, "[trainer] non-positive round-trip cost for "+excursion.Prefix, nil)
	}

	startIdx := tape.frameAt(uint64(fields["b_tick"]))

	if startIdx < 0 {
		return false, nil
	}

	endIdx := tape.frameAt(uint64(fields["c_tick"]))
	netReturn := pnl.Div(cost).Float64()
	holdSeconds := (fields["c_at"] - fields["b_at"]) / 1e9
	isUp := excursion.Role == "up"

	record := func(path string, count func(*pathTally)) {
		tally, ok := tallies[path]

		if !ok {
			tally = &pathTally{}
			tallies[path] = tally
		}

		count(tally)

		stat, ok := stats[path]

		if !ok {
			stat = &pathStats{}
			stats[path] = stat
		}

		stat.gain += netReturn
		stat.holdSeconds += holdSeconds
		stat.visits++
	}

	entryPath := tape.findPath(startIdx, max(startIdx-horizon, 0))

	if entryPath != "" {
		if isUp && netReturn > 0 {
			record(entryPath, func(tally *pathTally) { tally.ups++ })
		} else {
			record(entryPath, func(tally *pathTally) { tally.downs++ })
		}
	}

	if isUp && netReturn > 0 && endIdx >= 0 {
		if exitPath := tape.findPath(endIdx, startIdx); exitPath != "" && exitPath != entryPath {
			record(exitPath, func(tally *pathTally) { tally.exits++ })
		}
	}

	return true, nil
}

/*
publishPolicies replaces epoch's published policies with one Value per path
that has a majority action, written through the store with io.Copy.
*/
func (trainer *Trainer) publishPolicies(
	epoch uint64,
	tallies map[string]*pathTally,
	stats map[string]*pathStats,
) error {
	// Outputs of the epoch being regenerated are removed first; a failed
	// delete would mix stale and fresh policy, so it stops training.
	for _, prefix := range data.PolicyPrefixes(epoch) {
		if err := trainer.client.DeletePrefix(prefix); err != nil {
			return errnie.Err(errnie.IO, "[trainer] cannot clear "+prefix+" before regenerating it", err)
		}
	}

	counts := make(map[string]int)
	var sequence uint64

	for _, path := range slices.Sorted(maps.Keys(tallies)) {
		tally := tallies[path]
		totalVisits := tally.ups + tally.downs + tally.exits

		var action string

		switch {
		case tally.ups > tally.downs && tally.ups > tally.exits:
			action = "enter"
		case tally.exits > tally.ups && tally.exits >= tally.downs:
			action = "exit"
		case tally.downs >= tally.ups && tally.downs > tally.exits:
			action = "noop"
		}

		if action == "" {
			continue
		}

		entry := stats[path]

		if entry == nil || entry.visits == 0 {
			return errnie.Err(errnie.Internal, "[trainer] path "+path+" is tallied without stats", nil)
		}

		var confidence float64

		switch action {
		case "enter":
			confidence = float64(tally.ups) / float64(totalVisits)
		case "exit":
			confidence = float64(tally.exits) / float64(totalVisits)
		case "noop":
			confidence = float64(tally.downs) / float64(totalVisits)
		}

		sequence++

		val := &data.Value{
			Origin: data.PolicyOrigin,
			Role:   action,
			Scope:  data.PolicyScope,
			Epoch:  epoch,
			Tick:   sequence,
		}
		val.FormatPrefix()

		meanGain := entry.gain / float64(entry.visits)

		if err := data.AppendPolicy(val, path, []float64{
			meanGain,
			entry.holdSeconds / float64(entry.visits),
			float64(entry.visits),
			meanGain,
			confidence,
		}); err != nil {
			return err
		}

		if _, err := io.Copy(trainer.client, val); err != nil {
			return errnie.Err(errnie.IO, "[trainer] publishing "+val.Prefix, err)
		}

		counts[action]++
	}

	errnie.Info(fmt.Sprintf(
		"[trainer] published policies for epoch %d (enter=%d exit=%d noop=%d)",
		epoch, counts["enter"], counts["exit"], counts["noop"],
	))

	return nil
}
