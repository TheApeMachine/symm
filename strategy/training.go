package strategy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

/*
Training paper trades the live market against the CognitionTree: the
Sequitur sink advances every symbol's tape, asks the tree for the deepest
stored path the tape's tokens end on, and hands each matched action to Act,
which takes it through the paper Desk. Filled receives the desk's venue
executions; when a round trip closes, one paper record per traded decision
(data.PaperKeys) is io.Copy'd to the hub and appended to the local records
file. Training never writes storage.
*/
type Training struct {
	ctx     context.Context
	price   *broker.Price
	desk    *broker.Desk
	tree    *CognitionTree
	hub     io.Writer
	records io.Writer
	quote   string
	mu      sync.Mutex
	trips   map[string]*paperTrip
	seq     atomic.Uint64
}

/*
paperDecision is one traded decision and the fills it received.
*/
type paperDecision struct {
	action    string
	match     CognitionMatch
	intended  float64
	ordered   float64
	decidedAt time.Time
	qty       float64
	cost      float64
	fee       float64
	filledAt  time.Time
	filled    bool
}

/*
paperTrip is a symbol's open round trip: the entry decision and, once taken,
the exit decision.
*/
type paperTrip struct {
	entry *paperDecision
	exit  *paperDecision
}

/*
policy is one loaded generation of the published action tree: its keys, and the one
action each stored path agrees on ("" where its actions disagree).
*/
type policy struct {
	epoch   uint64
	gate    data.PolicyGate
	keys    []string
	actions map[string]string
	// metrics holds each key's stored data.PolicyKeys values.
	metrics map[string]map[string]float64
}

/*
NewTraining builds the paper trader. It refuses to run against anything but
the paper desk (market.model paper), and needs a non-empty tree, the hub and
the records file.
*/
func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
	tree *CognitionTree,
	hub io.Writer,
	records io.Writer,
) (*Training, error) {
	if err := errnie.Require(map[string]any{
		"price":   price,
		"desk":    desk,
		"tree":    tree,
		"hub":     hub,
		"records": records,
	}); err != nil {
		return nil, errnie.Err(errnie.Validation, "[training] cannot paper trade without price, desk, tree, hub and records", err)
	}

	if system.Cfg == nil || system.Cfg.Market.Model != "paper" {
		return nil, errnie.Err(errnie.Validation, "[training] market.model must be paper: run only paper trades", nil)
	}

	if tree.Empty() {
		return nil, errnie.Err(errnie.Validation, "[training] the cognition tree stores no paths", nil)
	}

	return &Training{
		ctx:     ctx,
		price:   price,
		desk:    desk,
		tree:    tree,
		hub:     hub,
		records: records,
		quote:   system.Cfg.Market.QuoteCurrency,
		trips:   make(map[string]*paperTrip),
	}, nil
}

/*
Open describes every round trip that has no completed exit: symbol, the
quantity held (filled or still pending) and the entry price. Such a trip
produces no record.
*/
func (training *Training) Open() []string {
	training.mu.Lock()
	defer training.mu.Unlock()

	open := make([]string, 0, len(training.trips))

	for symbol, trip := range training.trips {
		entry := trip.entry
		price, state := 0.0, "pending"

		if entry.qty > 0 {
			price = entry.cost / entry.qty
		}

		if entry.filled {
			state = "filled"
		}

		open = append(open, fmt.Sprintf(
			"%s qty %g of %g ordered (%s) entry %g, path %s",
			symbol, entry.qty, entry.ordered, state, price, entry.match.Path,
		))
	}

	slices.Sort(open)
	return open
}

/*
WarnOpen logs the round trips Open lists, loudly, at shutdown. No exit
record is fabricated for them.
*/
func (training *Training) WarnOpen() {
	if open := training.Open(); len(open) > 0 {
		errnie.Warn(fmt.Sprintf(
			"[training] %d paper positions still open at shutdown, not recorded: %s",
			len(open), strings.Join(open, "; "),
		))
	}
}

/*
Act takes a matched action for symbol through the paper desk and reports
what happened. A symbol holds at most one round trip: entering needs it
flat, exiting needs the entry this run made. Every failure is logged.
*/
func (training *Training) Act(symbol string, match CognitionMatch) string {
	switch match.Action {
	case "enter":
		return training.enter(symbol, match)
	case "exit":
		return training.exit(symbol, match)
	case "noop":
		return "noop: matched unpromising precursor"
	case "":
		return "no action: stored policies disagree on this path"
	}

	errnie.Error(errnie.Err(errnie.Validation, "[training] unknown action "+match.Action+" on "+match.Path, nil))
	return "ignored: unknown action " + match.Action
}

func (training *Training) enter(symbol string, match CognitionMatch) string {
	if training.desk.Holds(symbol) {
		return "holding: enter ignored"
	}

	_, ask := training.price.Touch(symbol)

	if ask == nil || ask.Sign() <= 0 {
		errnie.Error(errnie.Err(errnie.NotFound, "[training] no ask to enter "+symbol+" at", nil))
		return "enter failed: no ask"
	}

	cash := training.desk.Cash(training.quote)

	if cash == nil || cash.Sign() <= 0 {
		errnie.Error(errnie.Err(errnie.NotAcceptable, "[training] no "+training.quote+" cash on the paper desk to enter "+symbol, nil))
		return "enter failed: no cash"
	}

	entry, err := training.price.AllocateEntry(symbol, cash.SetScale(4).Mul(system.Cfg.Risk.MaxFraction))

	if err != nil {
		errnie.Error(errnie.Err(errnie.NotAcceptable, "[training] unable to size an entry for "+symbol, err))
		return "enter failed: " + err.Error()
	}

	// The decision is registered before the order goes out: its fills
	// arrive on the private ingress, possibly before Enter returns.
	training.mu.Lock()
	training.trips[symbol] = &paperTrip{entry: &paperDecision{
		action: "enter", match: match, intended: ask.Float64(), ordered: entry.Quantity.Float64(), decidedAt: time.Now(),
	}}
	training.mu.Unlock()

	if err := training.desk.Enter(symbol, entry.Quantity, match.Path); err != nil {
		training.mu.Lock()
		delete(training.trips, symbol)
		training.mu.Unlock()

		errnie.Error(errnie.Err(errnie.IO, "[training] enter failed for "+symbol, err))
		return "enter failed: " + err.Error()
	}

	return "enter submitted"
}

func (training *Training) exit(symbol string, match CognitionMatch) string {
	if !training.desk.Holds(symbol) {
		return "flat: exit ignored"
	}

	training.mu.Lock()
	trip := training.trips[symbol]
	entryFilled := trip != nil && trip.entry != nil && trip.entry.filled
	exitWorking := trip != nil && trip.exit != nil
	training.mu.Unlock()

	// A holding this run did not enter (restored from the paper account's
	// history) has no entry decision to account the round trip against.
	if trip == nil || trip.entry == nil || !entryFilled {
		errnie.Error(errnie.Err(errnie.NotAcceptable, "[training] "+symbol+" is held without a filled entry from this run: exit not taken", nil))
		return "exit refused: no filled entry this run"
	}

	if exitWorking {
		return "exiting: exit already working"
	}

	bid, _ := training.price.Touch(symbol)

	if bid == nil || bid.Sign() <= 0 {
		errnie.Error(errnie.Err(errnie.NotFound, "[training] no bid to exit "+symbol+" at", nil))
		return "exit failed: no bid"
	}

	training.mu.Lock()
	trip.exit = &paperDecision{action: "exit", match: match, intended: bid.Float64(), ordered: trip.entry.qty, decidedAt: time.Now()}
	training.mu.Unlock()

	if err := training.desk.Exit(symbol); err != nil {
		training.mu.Lock()
		trip.exit = nil
		training.mu.Unlock()

		errnie.Error(errnie.Err(errnie.IO, "[training] exit failed for "+symbol, err))
		return "exit failed: " + err.Error()
	}

	return "exit submitted"
}

/*
Filled accounts the desk's venue executions against the decisions this run
took. A fill for a symbol without a decision, or without its fee or venue
time, is an error: the record would not be what happened.
*/
func (training *Training) Filled(execution *kraken.Execution) error {
	var errs []error

	for _, row := range execution.Data {
		if row.LastQty == nil || row.LastQty.Sign() <= 0 {
			if status := strings.ToLower(row.OrderStatus); status == "canceled" || status == "rejected" || status == "expired" {
				errs = append(errs, training.drop(row))
			}

			continue
		}

		if err := training.fill(row); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (training *Training) fill(row kraken.ExecutionData) error {
	training.mu.Lock()
	defer training.mu.Unlock()

	trip := training.trips[row.Symbol]

	if trip == nil {
		return errnie.Err(errnie.NotFound, fmt.Sprintf("[training] %s fill for %s has no paper decision", row.Side, row.Symbol), nil)
	}

	decision := trip.entry

	if strings.EqualFold(row.Side, "sell") {
		decision = trip.exit
	}

	if decision == nil || decision.filled {
		return errnie.Err(errnie.NotFound, fmt.Sprintf("[training] %s fill for %s has no open %s decision", row.Side, row.Symbol, row.Side), nil)
	}

	if row.LastPrice == nil || row.LastPrice.Sign() <= 0 || row.FeeUsdEquiv == nil || row.Timestamp.IsZero() {
		return errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
			"[training] %s fill for %s lacks price, fee or venue time (order %s)", row.Side, row.Symbol, row.OrderID,
		), nil)
	}

	qty := row.LastQty.Float64()
	decision.qty += qty
	decision.cost += qty * row.LastPrice.Float64()
	decision.fee += row.FeeUsdEquiv.Float64()
	decision.filledAt = row.Timestamp

	// A decision is complete once its fills add up to what was ordered (to
	// the venue's lot rounding: 1e-8 base units).
	if decision.qty+1e-8 < decision.ordered*(1-1e-6) {
		return nil
	}

	decision.filled = true

	if decision == trip.exit {
		delete(training.trips, row.Symbol)
		return training.record(row.Symbol, trip)
	}

	return nil
}

/*
drop forgets the decision a canceled or rejected order carried, loudly.
*/
func (training *Training) drop(row kraken.ExecutionData) error {
	training.mu.Lock()
	defer training.mu.Unlock()

	trip := training.trips[row.Symbol]

	if trip != nil && strings.EqualFold(row.Side, "sell") {
		trip.exit = nil
	} else {
		delete(training.trips, row.Symbol)
	}

	return errnie.Err(errnie.IO, fmt.Sprintf(
		"[training] %s order %s for %s was %s: its decision is dropped", row.Side, row.OrderID, row.Symbol, row.OrderStatus,
	), nil)
}

/*
record emits the closed round trip as one paper record per decision.
*/
func (training *Training) record(symbol string, trip *paperTrip) error {
	entry, exit := trip.entry, trip.exit

	if math.Abs(exit.qty-entry.qty) > 1e-8+1e-6*entry.qty {
		return errnie.Err(errnie.Internal, fmt.Sprintf(
			"[training] %s exit filled %v of the %v entered", symbol, exit.qty, entry.qty,
		), nil)
	}

	entryPrice, exitPrice := entry.cost/entry.qty, exit.cost/exit.qty
	pnl := exit.cost - exit.fee - entry.cost - entry.fee
	netReturn := pnl / (entry.cost + entry.fee)

	for _, decision := range []*paperDecision{entry, exit} {
		fill := decision.cost / decision.qty
		slippage := (fill - decision.intended) / decision.intended

		if decision.action == "exit" {
			slippage = -slippage
		}

		value := data.NewValue(data.PaperOrigin, decision.action, symbol)
		value.Tick = training.seq.Add(1)
		value.FormatPrefix()

		if err := data.AppendPaper(value, decision.match.Path, training.tree.Epoch(), []float64{
			float64(decision.match.Depth), decision.intended, fill,
			float64(decision.decidedAt.UnixNano()), float64(decision.filledAt.UnixNano()),
			decision.filledAt.Sub(decision.decidedAt).Seconds(), slippage, entry.qty,
			entryPrice, exitPrice,
			float64(entry.filledAt.UnixNano()), float64(exit.filledAt.UnixNano()),
			entry.fee + exit.fee, pnl, netReturn,
		}); err != nil {
			return err
		}

		// The local file is the record; the hub is the view of it.
		if _, err := io.Copy(training.records, value); err != nil {
			return errnie.Err(errnie.IO, "[training] append paper record "+value.Prefix, err)
		}

		if _, err := io.Copy(training.hub, value); err != nil {
			return errnie.Err(errnie.IO, "[training] paper record "+value.Prefix+" to the hub", err)
		}
	}

	errnie.Info(fmt.Sprintf(
		"[training] %s round trip %s -> %s: pnl %.6f %s (net return %.5f)",
		symbol, entry.match.Path, exit.match.Path, pnl, training.quote, netReturn,
	))

	return nil
}

type GrammarRuleExport struct {
	ID        uint32   `json:"id"`
	Name      string   `json:"name"`
	Count     int      `json:"count"`
	Depth     int      `json:"depth"`
	Length    int      `json:"length"`
	Symbols   []string `json:"symbols"`
	Terminals []string `json:"terminals"`
	Action    string   `json:"action,omitempty"`
}

type GrammarExport struct {
	Symbol string              `json:"symbol"`
	Rules  []GrammarRuleExport `json:"rules"`
	Start  []string            `json:"start"`
}

/*
exportGrammar lists every rule of grammar with its references, depth, body
and expansion, and the action pol stores for that expansion as a path.
*/
func exportGrammar(symbol string, grammar *Grammar, pol *policy) GrammarExport {
	if grammar == nil {
		return GrammarExport{Symbol: symbol, Rules: []GrammarRuleExport{}, Start: []string{}}
	}

	rules := grammar.Rules()
	exports := make([]GrammarRuleExport, 0, len(rules))

	for _, rule := range rules {
		terms := rule.TerminalNames(grammar)
		action := ""

		if pol != nil {
			action = pol.actions[strings.Join(terms, "/")]
		}

		exports = append(exports, GrammarRuleExport{
			ID:        rule.ID(),
			Name:      RuleLabel(rule.ID()),
			Count:     rule.Count(),
			Depth:     rule.Depth(),
			Length:    rule.Length(),
			Symbols:   grammar.Symbols(rule),
			Terminals: terms,
			Action:    action,
		})
	}

	return GrammarExport{
		Symbol: symbol,
		Rules:  exports,
		Start:  grammar.Symbols(grammar.StartRule()),
	}
}

/*
loadPolicy reads the newest epoch the Trainer published: every stored path
with the one action its policies agree on ("" where they disagree). No
published epoch is an empty policy; a failed read or a malformed policy is
an error.
*/
func loadPolicy(reader *store.Reader) (*policy, error) {
	loaded := &policy{
		keys:    make([]string, 0),
		actions: make(map[string]string),
		metrics: make(map[string]map[string]float64),
	}

	for _, prefix := range data.PolicyPrefixes() {
		epochs, err := reader.Epochs(prefix)

		if err != nil {
			return nil, errnie.Err(errnie.IO, "[policy] listing epochs under "+prefix, err)
		}

		if len(epochs) > 0 {
			loaded.epoch = max(loaded.epoch, epochs[len(epochs)-1])
		}
	}

	if loaded.epoch == 0 {
		return loaded, nil
	}

	for val, err := range reader.Values(data.PolicyPrefixes(loaded.epoch)...) {
		if err != nil {
			// A failed download is the store failing (IO); a stored Value that
			// does not decode is malformed data, not an unreachable store.
			kind := errnie.UnprocessableContent

			if errnie.IsIO(err) {
				kind = errnie.IO
			}

			return nil, errnie.Err(kind, fmt.Sprintf("[policy] reading epoch %d", loaded.epoch), err)
		}

		gate, err := data.ReadPolicyGate(val)

		if err != nil {
			return nil, err
		}

		if loaded.gate.Epoch == 0 {
			loaded.gate = gate
		} else if !gate.Same(loaded.gate) || gate.TickEpoch != loaded.gate.TickEpoch {
			return nil, errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
				"[policy] %s carries %s, other policies of the epoch %s", val.Prefix, gate, loaded.gate,
			), nil)
		}

		action := val.Role
		pathKey, err := data.PolicyPath(val)

		if err != nil {
			return nil, err
		}

		if existing, found := loaded.actions[pathKey]; found && existing != action {
			action = ""
		}

		key := pathKey + "/" + val.Role + ".bin"
		metrics := data.Metrics(val)

		for _, name := range data.PolicyKeys {
			if _, ok := metrics[name]; !ok {
				return nil, errnie.Err(errnie.UnprocessableContent, "[policy] "+val.Prefix+" has no "+name, nil)
			}
		}

		loaded.actions[pathKey] = action
		loaded.metrics[key] = metrics
		loaded.keys = append(loaded.keys, key)
	}

	slices.Sort(loaded.keys)
	loaded.keys = slices.Compact(loaded.keys)

	return loaded, nil
}
