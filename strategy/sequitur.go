package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/types"
)

/*
Sequitur is the dashboard's live view of the grammar the Trainer induces. It
is a workspace sink: every completed tick is io.Copy'd into it, each symbol's
grid frame advances that symbol's tape exactly as the Trainer's does
(symbolTape.advance), and for the focused symbol it io.Copy's one
SequiturFrame to the hub per new chord. It never trades and never stores.
*/
type Sequitur struct {
	ctx    context.Context
	hub    io.Writer
	grid   *nmstore.Grid
	policy *policy
	tapes  map[string]*symbolTape
	held   map[string]int
}

/*
SequiturRegion is one active grid region and its brightness this frame.
*/
type SequiturRegion struct {
	Key        uint8   `json:"key"`
	Brightness float64 `json:"brightness"`
}

/*
SequiturPolicy is how the frame's path meets the published policy: the
epoch loaded and its path count, whether the path is stored, and the action
it maps to ("" when stored policies disagree on it).
*/
type SequiturPolicy struct {
	Epoch  uint64 `json:"epoch"`
	Paths  int    `json:"paths"`
	Stored bool   `json:"stored"`
	Action string `json:"action"`
}

/*
SequiturFrame is one new chord appended to a symbol's grammar: the gate that
produced it, every structural event it caused, the rule it completed, the
active path and its policy, the whole grammar after it, and the invariants
checked over that grammar. Held counts the frames since the previous chord
that repeated it (nothing is appended for those).
*/
type SequiturFrame struct {
	Symbol         string              `json:"symbol"`
	Epoch          uint64              `json:"epoch"`
	Tick           uint64              `json:"tick"`
	Timestamp      uint64              `json:"timestamp"`
	Active         []SequiturRegion    `json:"active"`
	Midpoint       float64             `json:"midpoint"`
	Fired          []string            `json:"fired"`
	Held           int                 `json:"held"`
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
NewSequitur loads the newest published policy through client and streams
frames into hub.
*/
func NewSequitur(ctx context.Context, hub io.Writer, client *store.Client) (*Sequitur, error) {
	if hub == nil || client == nil {
		return nil, errnie.Err(errnie.Validation, "[sequitur] needs the hub and a store client", nil)
	}

	loaded, err := loadPolicy(client)

	if err != nil {
		return nil, errnie.Err(errnie.IO, "[sequitur] unable to load the published policy", err)
	}

	return &Sequitur{
		ctx:    ctx,
		hub:    hub,
		grid:   nmstore.NewGrid(ctx),
		policy: loaded,
		tapes:  make(map[string]*symbolTape),
		held:   make(map[string]int),
	}, nil
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
		frame, err := sequitur.grid.Observe(tick, symbol)

		if err != nil {
			return 0, errnie.Err(errnie.Internal, "[sequitur] grid frame of "+symbol, err)
		}

		if len(frame.Regions) == 0 {
			continue
		}

		tape, ok := sequitur.tapes[symbol]

		if !ok {
			tape = newSymbolTape()
			tape.grammar.Trace()
			sequitur.tapes[symbol] = tape
		}

		step, ok := tape.advance(frame.Regions)
		events := tape.grammar.Drain()

		if !ok {
			continue
		}

		if step.Repeat {
			sequitur.held[symbol]++
			continue
		}

		held := sequitur.held[symbol]
		sequitur.held[symbol] = 0

		if symbol != focus {
			continue
		}

		if err := sequitur.emit(tick, symbol, tape, step, events, held); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

func (sequitur *Sequitur) emit(
	tick *data.Value, symbol string, tape *symbolTape, step tapeStep, events []GrammarEvent, held int,
) error {
	export := exportGrammar(symbol, tape.grammar, sequitur.policy)
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
		Events:     events,
		Path:       step.Path,
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

	action, stored := sequitur.policy.actions[step.Path]
	frame.Policy = SequiturPolicy{
		Epoch:  sequitur.policy.epoch,
		Paths:  len(sequitur.policy.actions),
		Stored: stored,
		Action: action,
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
