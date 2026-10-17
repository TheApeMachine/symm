package strategy

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

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
Policies are published under data.PolicyPrefixes(policy epoch): a fresh
policy epoch per trained tick epoch, one Value per learned path, pooled
across the epoch's symbols, each carrying its data.PolicyGate.
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

/*
trainFunnel counts what became of one trained epoch's up/down excursions,
stage by stage, and of the chords the midpoint rule proposed, so the summary
says where excursions were lost instead of only what was published.
*/
type trainFunnel struct {
	// Chords: new chords the midpoint rule proposed, those the surprise gate
	// blocked, and of those how many it blocked while the symbol was cold,
	// by reason (coldSilent, coldHistory, coldFalling).
	proposed int
	blocked  int
	cold     map[string]int

	// Symbols the epoch replayed, and how many of them warmed up (their
	// coder's error settled); all of them with the gate off.
	symbols int
	warm    int

	// Excursions: up/down excursions, those with no frame of the symbol at
	// or before the entry tick, those with frames but no appended chord
	// within learning.precursor_horizon frames, and those with a path.
	excursions int
	noFrame    int
	noPath     int
	withPath   int

	// Votes the excursions with a path cast: enter (a profitable up), noop
	// (a down or a losing up) and exit (a profitable up's path at exit).
	enters int
	noops  int
	exits  int

	// Paths: distinct voted paths, and those whose votes tie so no action
	// holds a majority (nothing is published for them).
	paths int
	tied  int
}

func (funnel *trainFunnel) String() string {
	return fmt.Sprintf(
		"funnel: symbols=%d warmed up=%d | chords proposed=%d blocked by gate=%d (cold: %d no error yet, %d too little history, %d error still falling) | excursions=%d no frame before entry=%d no path within horizon=%d with path=%d | votes enter=%d noop=%d exit=%d | paths=%d tied (unpublished)=%d",
		funnel.symbols, funnel.warm, funnel.proposed, funnel.blocked, funnel.cold[coldSilent], funnel.cold[coldHistory], funnel.cold[coldFalling],
		funnel.excursions, funnel.noFrame, funnel.noPath, funnel.withPath,
		funnel.enters, funnel.noops, funnel.exits,
		funnel.paths, funnel.tied,
	)
}

type frameRecord struct {
	tick uint64
	path string
}

/*
symbolTape is the grammar state of one symbol's grid frames over an epoch,
and the gate's running surprise moments for the symbol.
*/
type symbolTape struct {
	grammar      *Grammar
	gate         *chordGate
	surprise     surpriseMoments
	lastSym      string
	recentTokens []string
	frames       []frameRecord

	// proposed, blocked and cold count the train path's new chords, those
	// the gate blocked, and the blocked ones while the symbol was cold, by
	// reason.
	proposed int
	blocked  int
	cold     map[string]int
}

func newSymbolTape(gate *chordGate) *symbolTape {
	return &symbolTape{grammar: NewGrammar(), gate: gate, recentTokens: make([]string, 0, 16)}
}

/*
tapeStep is what one grid frame did to a symbol's grammar: the active regions
and the midpoint gate over them, the region tokens that fired, whether that
chord merely repeated the previous one (nothing is appended then), what the
surprise gate read (nil when it is off) and whether it blocked the chord
(nothing is appended then either), the rule the append completed, and the
grammar path active after it. The trainer tallies excursions against Path;
live inspection shows every field.
*/
type tapeStep struct {
	Active   []*nmstore.Region
	Midpoint float64
	Fired    []string
	Repeat   bool
	Gate     *GateReading
	Blocked  bool
	Rule     *Rule
	Path     string
}

/*
step advances the tape by one stored tick of its symbol, the same way for
train and run: the grid frame, the gate's solver step (every tick, chord
or not, so its surprise is causal per symbol) and advance.
*/
func (tape *symbolTape) step(grid *nmstore.Grid, tick *data.Value, symbol *broker.SymbolTick) (tapeStep, bool, error) {
	frame, err := grid.Observe(tick, symbol.Symbol)

	if err != nil {
		return tapeStep{}, false, errnie.Err(errnie.Internal, "[tape] grid frame of "+symbol.Symbol+" at "+tick.Prefix, err)
	}

	gate, err := tape.gate.read(&tape.surprise, tick, symbol)

	if err != nil {
		return tapeStep{}, false, err
	}

	step, ok := tape.advance(frame.Regions, gate)
	return step, ok, nil
}

/*
advance turns one frame's regions into region tokens and appends them to the
grammar, unless gate (nil when the gate is off) did not pass. It reports
false when no region is active, so the frame has no chord.
*/
func (tape *symbolTape) advance(regions []*nmstore.Region, gate *GateReading) (tapeStep, bool) {
	step := tapeStep{Active: make([]*nmstore.Region, 0), Gate: gate}

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

	// The midpoint rule proposed a new chord; the surprise gate decides.
	if gate != nil && !gate.Passed {
		step.Blocked = true
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
observe advances the tape by one stored tick of its symbol and records the
grammar path active at that tick (empty on a repeated or blocked chord).
*/
func (tape *symbolTape) observe(grid *nmstore.Grid, tick *data.Value, symbol *broker.SymbolTick) error {
	step, ok, err := tape.step(grid, tick, symbol)

	if err != nil || !ok {
		return err
	}

	if !step.Repeat {
		tape.proposed++
	}

	if step.Blocked {
		tape.blocked++

		if !step.Gate.Warm {
			if tape.cold == nil {
				tape.cold = make(map[string]int)
			}

			tape.cold[step.Gate.Cold]++
		}
	}

	tape.frames = append(tape.frames, frameRecord{tick: tick.Tick, path: step.Path})
	return nil
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
tapeReplay is the train path's tape per symbol over one epoch's stored ticks,
in the order symbols first trade, limited to symbols when any are given.
*/
type tapeReplay struct {
	grid    *nmstore.Grid
	gate    *chordGate
	symbols []string
	tapes   map[string]*symbolTape
	order   []string
}

func newTapeReplay(ctx context.Context, gate *chordGate, symbols []string) *tapeReplay {
	return &tapeReplay{
		grid: nmstore.NewGrid(ctx), gate: gate, symbols: symbols,
		tapes: make(map[string]*symbolTape), order: make([]string, 0),
	}
}

/*
observe advances every symbol of one stored tick.
*/
func (replay *tapeReplay) observe(value *data.Value) error {
	symbolTicks, err := broker.ReadTick(value)

	if err != nil {
		return err
	}

	for _, symbolTick := range symbolTicks {
		symbol := symbolTick.Symbol

		if len(replay.symbols) > 0 && !slices.Contains(replay.symbols, symbol) {
			continue
		}

		tape, ok := replay.tapes[symbol]

		if !ok {
			tape = newSymbolTape(replay.gate)
			replay.tapes[symbol] = tape
			replay.order = append(replay.order, symbol)
		}

		if err := tape.observe(replay.grid, value, symbolTick); err != nil {
			return err
		}
	}

	return nil
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
	// One gate per trained epoch: each epoch replays from a cold solver,
	// exactly as `make run` starts one.
	gate, err := newChordGate(trainer.Context(), system.Cfg.Resonance)

	if err != nil {
		return err
	}

	watermark, err := readWatermark(trainer.client, epoch)

	if err != nil {
		return err
	}

	replay := newTapeReplay(trainer.Context(), gate, trainer.symbols)

	grown, err := replayThrough(trainer.client, watermark, func(value *data.Value) error {
		if err := trainer.Context().Err(); err != nil {
			return err
		}

		return replay.observe(value)
	})

	if err != nil {
		return err
	}

	tapes, order := replay.tapes, replay.order
	ticks, firstTick, lastTick := watermark.Ticks, watermark.FirstTick, watermark.LastTick

	if grown > 0 {
		errnie.Warn(fmt.Sprintf(
			"[trainer] tick epoch %d grew since detect: tick %d is stored past its watermark; training on ticks %d..%d only (re-run detect to train on more)",
			epoch, grown, firstTick, lastTick,
		))
	}

	tallies := make(map[string]*pathTally)
	stats := make(map[string]*pathStats)
	funnel := &trainFunnel{cold: make(map[string]int)}

	for _, symbol := range order {
		tape := tapes[symbol]
		funnel.symbols++
		funnel.proposed += tape.proposed

		if gate.solver == nil || tape.surprise.settle.warm {
			funnel.warm++
		}

		funnel.blocked += tape.blocked

		for reason, count := range tape.cold {
			funnel.cold[reason] += count
		}

		for excursion, err := range store.Values(trainer.client, data.ExcursionPrefixes(symbol, epoch)...) {
			if err != nil {
				return err
			}

			if excursion.Role != "up" && excursion.Role != "down" {
				continue
			}

			funnel.excursions++

			if err := trainer.tally(symbol, tape, excursion, horizon, tallies, stats, funnel); err != nil {
				return err
			}
		}
	}

	if funnel.excursions == 0 {
		return errnie.Err(errnie.NotFound, fmt.Sprintf(
			"[trainer] epoch %d has no up/down excursions for %d symbols: run detect on it first", epoch, len(order),
		), nil)
	}

	settings := gate.settings
	settings.Epoch, settings.TickEpoch = uint64(time.Now().UnixNano()), epoch

	errnie.Info(fmt.Sprintf(
		"[trainer] epoch %d: %d ticks (tick %d..%d), %d symbols, %d up/down excursions, %s; %s",
		epoch, ticks, firstTick, lastTick, len(order), funnel.excursions, settings, watermark,
	))

	return trainer.publishPolicies(settings, tallies, stats, funnel)
}

/*
readWatermark is epoch's detector watermark; none (detect never completed
on the epoch) or more than one is an error.
*/
func readWatermark(client *store.Client, epoch uint64) (data.DetectorWatermark, error) {
	var found []data.DetectorWatermark

	for value, err := range store.Values(client, data.DetectorWatermarkPrefix(epoch)) {
		if err != nil {
			return data.DetectorWatermark{}, err
		}

		watermark, err := data.ReadDetectorWatermark(value)

		if err != nil {
			return data.DetectorWatermark{}, err
		}

		found = append(found, watermark)
	}

	switch len(found) {
	case 0:
		return data.DetectorWatermark{}, errnie.Err(errnie.NotFound, fmt.Sprintf(
			"[trainer] epoch %d has no detector watermark under %s: run detect on it (to completion) first",
			epoch, data.DetectorWatermarkPrefix(epoch),
		), nil)
	case 1:
		return found[0], nil
	}

	return data.DetectorWatermark{}, errnie.Err(errnie.Conflict, fmt.Sprintf(
		"[trainer] epoch %d has %d detector watermarks under %s", epoch, len(found), data.DetectorWatermarkPrefix(epoch),
	), nil)
}

/*
replayThrough feeds observe the epoch's stored ticks up to and including
watermark.LastTick, in order, and checks they are exactly the ticks detect
read (the same first tick and count). It returns the first stored tick past
the watermark, 0 when the epoch has not grown since detect.
*/
func replayThrough(client *store.Client, watermark data.DetectorWatermark, observe func(*data.Value) error) (uint64, error) {
	ticks := 0
	var firstTick, grown uint64

	for value, err := range store.Values(client, data.TickPrefix(watermark.Epoch)) {
		if err != nil {
			return 0, err
		}

		if value.Tick > watermark.LastTick {
			grown = value.Tick
			break
		}

		if ticks == 0 {
			firstTick = value.Tick
		}

		if err := observe(value); err != nil {
			return 0, err
		}

		ticks++
	}

	if ticks != watermark.Ticks || firstTick != watermark.FirstTick {
		return 0, errnie.Err(errnie.Conflict, fmt.Sprintf(
			"[trainer] epoch %d stores %d ticks from tick %d up to the watermark, but detect read %d from tick %d (%s): re-run detect",
			watermark.Epoch, ticks, firstTick, watermark.Ticks, watermark.FirstTick, watermark,
		), nil)
	}

	return grown, nil
}

/*
tally attributes one excursion to the grammar paths active at its entry and
at its exit: the points detect pulled back from ignition B and exhaustion C
so orders placed there can fill (data.ExcursionKeys). The long round trip
buys at the ask the entry tick recorded and sells at the exit tick's bid. It
counts in funnel which stage the excursion reached: no frame before the
entry, no appended chord within horizon frames of it, or a path and the
votes it cast.
*/
func (trainer *Trainer) tally(
	symbol string,
	tape *symbolTape,
	excursion *data.Value,
	horizon int,
	tallies map[string]*pathTally,
	stats map[string]*pathStats,
	funnel *trainFunnel,
) error {
	fields, err := data.ExcursionFields(excursion)

	if err != nil {
		return err
	}

	pnl, cost, err := trainer.price.RoundTrip(
		symbol, decimal.NewFromFloat64(fields["entry_ask"]), decimal.NewFromFloat64(fields["exit_bid"]),
	)

	if err != nil {
		return errnie.Err(errnie.Validation, "[trainer] round trip of "+excursion.Prefix, err)
	}

	if cost.Sign() <= 0 {
		return errnie.Err(errnie.Validation, "[trainer] non-positive round-trip cost for "+excursion.Prefix, nil)
	}

	startIdx := tape.frameAt(uint64(fields["entry_tick"]))

	if startIdx < 0 {
		funnel.noFrame++
		return nil
	}

	endIdx := tape.frameAt(uint64(fields["exit_tick"]))
	netReturn := pnl.Div(cost).Float64()
	holdSeconds := (fields["exit_at"] - fields["entry_at"]) / 1e9
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

	if entryPath == "" {
		funnel.noPath++
		return nil
	}

	funnel.withPath++

	if isUp && netReturn > 0 {
		funnel.enters++
		record(entryPath, func(tally *pathTally) { tally.ups++ })
	} else {
		funnel.noops++
		record(entryPath, func(tally *pathTally) { tally.downs++ })
	}

	if isUp && netReturn > 0 && endIdx >= 0 {
		if exitPath := tape.findPath(endIdx, startIdx); exitPath != "" && exitPath != entryPath {
			funnel.exits++
			record(exitPath, func(tally *pathTally) { tally.exits++ })
		}
	}

	return nil
}

/*
majority is the action a path's votes elect, or "" when they tie and no
action holds a majority.
*/
func (tally *pathTally) majority() string {
	switch {
	case tally.ups > tally.downs && tally.ups > tally.exits:
		return "enter"
	case tally.exits > tally.ups && tally.exits >= tally.downs:
		return "exit"
	case tally.downs >= tally.ups && tally.downs > tally.exits:
		return "noop"
	}

	return ""
}

/*
publishPolicies publishes one Value per path that has a majority action
under the fresh policy epoch gate.Epoch, each tagged with gate, written
through the store with io.Copy. Earlier policy epochs are left as they are,
so trainings of the same ticks under different gates can be compared.
*/
func (trainer *Trainer) publishPolicies(
	gate data.PolicyGate,
	tallies map[string]*pathTally,
	stats map[string]*pathStats,
	funnel *trainFunnel,
) error {
	epoch := gate.Epoch

	for _, prefix := range data.PolicyPrefixes(epoch) {
		keys, err := trainer.client.ListKeys(prefix)

		if err != nil {
			return errnie.Err(errnie.IO, "[trainer] listing "+prefix, err)
		}

		if len(keys) > 0 {
			return errnie.Err(errnie.Conflict, fmt.Sprintf("[trainer] policy epoch %d already holds %d policies under %s", epoch, len(keys), prefix), nil)
		}
	}

	counts := make(map[string]int)
	var sequence uint64

	for _, path := range slices.Sorted(maps.Keys(tallies)) {
		tally := tallies[path]
		totalVisits := tally.ups + tally.downs + tally.exits
		action := tally.majority()
		funnel.paths++

		if action == "" {
			funnel.tied++
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

		if err := data.AppendPolicyGate(val, gate); err != nil {
			return err
		}

		if _, err := io.Copy(trainer.client, val); err != nil {
			return errnie.Err(errnie.IO, "[trainer] publishing "+val.Prefix, err)
		}

		counts[action]++
	}

	errnie.Info(fmt.Sprintf(
		"[trainer] published policy epoch %d for tick epoch %d (enter=%d exit=%d noop=%d); %s",
		epoch, gate.TickEpoch, counts["enter"], counts["exit"], counts["noop"], funnel,
	))

	return nil
}
