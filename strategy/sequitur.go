package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Sequitur is the dashboard's live view of the grammar the Trainer induces. It
is a workspace sink: every completed tick is io.Copy'd into it, each symbol's
grid frame advances that symbol's tape exactly as the Trainer's does
(symbolTape.advance). After every new chord the CognitionTree is asked for
the deepest stored path the symbol's recent tokens end on; when paper
trading is on (Trade), the matched action goes to Training.Act. For the
focused symbol it io.Copy's one SequiturFrame to the hub per new chord. It
never stores.
*/
type Sequitur struct {
	ctx    context.Context
	hub    io.Writer
	grid   *nmstore.Grid
	gate   *chordGate
	tree   *CognitionTree
	tapes  map[string]*symbolTape
	tokens map[string][]string
	held   map[string]int
	trader atomic.Pointer[Training]
}

/*
SequiturRegion is one active grid region and its brightness this frame.
*/
type SequiturRegion struct {
	Key        uint8   `json:"key"`
	Brightness float64 `json:"brightness"`
}

/*
SequiturPolicy is how the symbol's recent tokens meet the CognitionTree: the
policy epoch loaded (0 when none is published) and its path count, whether
paper trading is on, whether a stored path matched, the matched path, its
depth in tokens and its action ("" when stored policies disagree on it),
and what paper trading did with it ("" when it is off or nothing matched).
*/
type SequiturPolicy struct {
	Epoch   uint64 `json:"epoch,string"`
	Paths   int    `json:"paths"`
	Trading bool   `json:"trading"`
	Stored  bool   `json:"stored"`
	Match   string `json:"match"`
	Depth   int    `json:"depth"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
}

/*
SequiturFrame is one new chord appended to a symbol's grammar: the gate that
produced it, every structural event it caused, the rule it completed, the
active path and its policy, the whole grammar after it, and the invariants
checked over that grammar. Epochs and timestamps are JSON strings: they
exceed what a JavaScript number holds exactly. Held counts the frames since the previous chord
that repeated it (nothing is appended for those). Gate is what the surprise
gate read on this tick (null when resonance.gate is off); Blocked frames are
chords the gate did not pass: nothing was appended and nothing matched.
*/
type SequiturFrame struct {
	Symbol         string              `json:"symbol"`
	Epoch          uint64              `json:"epoch,string"`
	Tick           uint64              `json:"tick"`
	Timestamp      uint64              `json:"timestamp,string"`
	Active         []SequiturRegion    `json:"active"`
	Midpoint       float64             `json:"midpoint"`
	Fired          []string            `json:"fired"`
	Held           int                 `json:"held"`
	Gate           *GateReading        `json:"gate"`
	Blocked        bool                `json:"blocked"`
	Events         []GrammarEvent      `json:"events"`
	Completed      string              `json:"completed"`
	Path           string              `json:"path"`
	Policy         SequiturPolicy      `json:"policy"`
	Rules          []GrammarRuleExport `json:"rules"`
	Start          []string            `json:"start"`
	RawLength      int                 `json:"rawLength"`
	GrammarSymbols int                 `json:"grammarSymbols"`
	Violations     []string            `json:"violations"`
}

/*
NewSequitur streams frames into hub, matching tapes against tree. Its tapes
gate chords by system.Cfg.Resonance, the settings tree's policies must have
been trained under (LoadCognitionTree refuses any other).
*/
func NewSequitur(ctx context.Context, hub io.Writer, tree *CognitionTree) (*Sequitur, error) {
	if hub == nil || tree == nil {
		return nil, errnie.Err(errnie.Validation, "[sequitur] needs the hub and a cognition tree", nil)
	}

	gate, err := newChordGate(ctx, system.Cfg.Resonance)

	if err != nil {
		return nil, err
	}

	if !tree.Empty() && !tree.Gate().Same(gate.settings) {
		return nil, errnie.Err(errnie.Validation, fmt.Sprintf(
			"[sequitur] policy epoch %d was trained under %s, this run gates by %s", tree.Epoch(), tree.Gate(), gate.settings,
		), nil)
	}

	return &Sequitur{
		ctx:    ctx,
		hub:    hub,
		grid:   nmstore.NewGrid(ctx),
		gate:   gate,
		tree:   tree,
		tapes:  make(map[string]*symbolTape),
		tokens: make(map[string][]string),
		held:   make(map[string]int),
	}, nil
}

/*
Trade turns paper trading on: from the next tick, every matched action goes
to training.Act.
*/
func (sequitur *Sequitur) Trade(training *Training) {
	sequitur.trader.Store(training)
}

/*
Write reads one serialized tick and advances every symbol it carries.
*/
func (sequitur *Sequitur) Write(p []byte) (int, error) {
	tick := data.New()

	if _, err := tick.Write(p); err != nil {
		return 0, errnie.Err(errnie.Validation, "[sequitur] unreadable tick", err)
	}

	symbolTicks, err := broker.ReadTick(tick)

	if err != nil {
		return 0, errnie.Err(errnie.Validation, "[sequitur] tick "+tick.Prefix, err)
	}

	focus := types.Focus()

	for _, symbolTick := range symbolTicks {
		symbol := symbolTick.Symbol
		tape, ok := sequitur.tapes[symbol]

		if !ok {
			tape = newSymbolTape(sequitur.gate)
			tape.grammar.Trace()
			sequitur.tapes[symbol] = tape
		}

		step, ok, err := tape.step(sequitur.grid, tick, symbolTick)

		if err != nil {
			return 0, errnie.Err(errnie.Internal, "[sequitur] tape of "+symbol, err)
		}

		events := tape.grammar.Drain()

		if !ok {
			continue
		}

		if step.Repeat {
			sequitur.held[symbol]++
			continue
		}

		// A chord the gate blocked appends nothing and matches nothing; the
		// focused symbol's frame still shows what the gate read.
		if step.Blocked {
			if symbol == focus {
				if err := sequitur.emit(tick, symbol, tape, step, events, sequitur.held[symbol], sequitur.policy(false, CognitionMatch{}, nil, "")); err != nil {
					return 0, err
				}
			}

			continue
		}

		held := sequitur.held[symbol]
		sequitur.held[symbol] = 0

		tokens := append(sequitur.tokens[symbol], step.Fired...)
		tokens = tokens[max(0, len(tokens)-sequitur.tree.Depth()):]
		sequitur.tokens[symbol] = tokens

		match, matched := sequitur.tree.Match(tokens)
		trader := sequitur.trader.Load()
		outcome := ""

		if matched && trader != nil {
			outcome = trader.Act(symbol, match)
		}

		// Every symbol's live match, for the cognitive tree tab.
		if !sequitur.tree.Empty() {
			if err := writeCognition(sequitur.hub, data.CognitionMatchRole, symbol, CognitionMatchFrame{
				Symbol: symbol, Tick: tick.Tick, Tokens: tokens, Stored: matched,
				Match: match.Path, Depth: match.Depth, Action: match.Action,
			}, tick.Tick, tick.Timestamp); err != nil {
				return 0, err
			}
		}

		if symbol != focus {
			continue
		}

		policy := sequitur.policy(matched, match, trader, outcome)

		if err := sequitur.emit(tick, symbol, tape, step, events, held, policy); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

/*
policy is how the recent tokens met the tree on one frame.
*/
func (sequitur *Sequitur) policy(matched bool, match CognitionMatch, trader *Training, outcome string) SequiturPolicy {
	return SequiturPolicy{
		Epoch:   sequitur.tree.Epoch(),
		Paths:   sequitur.tree.Paths(),
		Trading: trader != nil,
		Stored:  matched,
		Match:   match.Path,
		Depth:   match.Depth,
		Action:  match.Action,
		Outcome: outcome,
	}
}

func (sequitur *Sequitur) emit(
	tick *data.Value, symbol string, tape *symbolTape, step tapeStep, events []GrammarEvent, held int, policy SequiturPolicy,
) error {
	export := exportGrammar(symbol, tape.grammar, sequitur.tree.policy)
	start := tape.grammar.StartRule()

	frame := SequiturFrame{
		Symbol:     symbol,
		Epoch:      tick.Epoch,
		Tick:       tick.Tick,
		Timestamp:  tick.Timestamp,
		Active:     make([]SequiturRegion, 0, len(step.Active)),
		Midpoint:   step.Midpoint,
		Fired:      step.Fired,
		Held:       held,
		Gate:       step.Gate,
		Blocked:    step.Blocked,
		Events:     events,
		Path:       step.Path,
		Policy:     policy,
		Rules:      export.Rules,
		Start:      export.Start,
		RawLength:  len(start.Terminals()),
		Violations: tape.grammar.Check(),
	}

	if frame.Events == nil {
		frame.Events = []GrammarEvent{}
	}

	for _, region := range step.Active {
		frame.Active = append(frame.Active, SequiturRegion{Key: region.Key, Brightness: region.Brightness})
	}

	if step.Rule != nil {
		frame.Completed = RuleLabel(step.Rule.ID())
	}

	frame.GrammarSymbols = start.Length()

	for _, rule := range export.Rules {
		frame.GrammarSymbols += rule.Length
	}

	buf, err := json.Marshal(frame)

	if err != nil {
		return errnie.Err(errnie.Internal, "[sequitur] frame of "+symbol, err)
	}

	out := data.NewValue(data.SequiturOrigin, data.SequiturRole, symbol)
	out.Epoch, out.Tick, out.Timestamp = tick.Epoch, tick.Tick, tick.Timestamp
	out.FormatPrefix()
	out.AppendJSON(buf, data.LayerGrammar)

	if _, err := io.Copy(sequitur.hub, out); err != nil {
		return errnie.Err(errnie.IO, fmt.Sprintf("[sequitur] frame of %s at tick %d to the hub", symbol, tick.Tick), err)
	}

	return nil
}
