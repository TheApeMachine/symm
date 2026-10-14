package strategy

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

/*
Training paper trades the live market against the action tree the Trainer
published: every frame becomes a region token, the token advances the
symbol's grammar through Sequitur, and when an induced rule matches a stored
path with an agreed action, that action is taken through the Desk.
*/
type Training struct {
	*runtime.System
	price    *broker.Price
	desk     *broker.Desk
	client   *store.Client
	grid     *nmstore.Grid
	policy   atomic.Pointer[policy]
	grammars map[string]*Grammar
	lastSym  map[string]string
	paths    map[string][]string
	last     map[string]*data.Measurement
}

/*
policy is one loaded generation of the published action tree: its keys, and the one
action each stored path agrees on ("" where its actions disagree). It is
swapped whole.
*/
type policy struct {
	keys    []string
	actions map[string]string
}

/*
Stored path actions: the final segment of every key.
*/
const (
	actionEnter = "enter.bin"
	actionExit  = "exit.bin"
	actionNoop  = "noop.bin"
)

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
	client *store.Client,
) *Training {
	training := &Training{
		System:   runtime.NewSystem(ctx, "training", price),
		price:    price,
		desk:     desk,
		client:   client,
		grid:     nmstore.NewGrid(ctx),
		grammars: make(map[string]*Grammar),
		lastSym:  make(map[string]string),
		paths:    make(map[string][]string),
		last:     make(map[string]*data.Measurement),
	}

	training.policy.Store(&policy{
		keys:    make([]string, 0),
		actions: make(map[string]string),
	})

	if err := errnie.Require(map[string]any{
		"price":  price,
		"desk":   desk,
		"client": client,
	}); err != nil {
		training.Error(errnie.Err(
			errnie.Validation, "[training] cannot paper trade without price, desk and client", err,
		))

		return training
	}

	if err := training.ReloadKeys(ctx); err != nil {
		training.Error(errnie.Err(
			errnie.IO, "[training] unable to load the published action tree", err,
		))

		return training
	}

	training.Transition(runtime.INIT)
	return training
}

/*
CognitionTree exports the stored S3 token paths for the learning dashboard.
*/
func (training *Training) CognitionTree() ui.CognitionTreeExport {
	return ui.CognitionTreeExport{
		Keys: training.policy.Load().keys,
	}
}

/*
Step receives the live market join, scores its regions through the grid,
and advances the symbol's grammar, acting when an induced rule matches a
stored path with an agreed action.
*/
func (training *Training) Step(prior *data.Measurement) *data.Measurement {
	regions := training.grid.Observe(prior)
	metrics := make(map[string]float64, len(regions)*2)

	for _, region := range regions {
		metrics[fmt.Sprintf("region_brightness:R%02d", region.Key)] = region.Brightness
		metrics[fmt.Sprintf("region_members:R%02d", region.Key)] = float64(region.Counts)
	}

	training.inspectPosition(prior.Label, metrics)

	emit := func(meta ...*data.StringEntry) *data.Measurement {
		prev := training.last[prior.Label]
		if prev == nil {
			prev = data.NewMeasurement(prior.Epoch, prior.Label, training.Name(), prior.At, prior.From)
		}

		next := prev.Next(prior, metrics, meta...)
		training.last[prior.Label] = next
		return next
	}

	current := training.policy.Load()

	if len(current.actions) == 0 || len(regions) == 0 {
		return emit()
	}

	grammar, exists := training.grammars[prior.Label]
	if !exists {
		grammar = NewGrammar()
		training.grammars[prior.Label] = grammar
	}

	sym := fmt.Sprintf("R%02d", regions[0].Key)
	if sym == training.lastSym[prior.Label] {
		return emit()
	}

	training.lastSym[prior.Label] = sym
	rule := grammar.Append(sym)
	if rule == nil {
		return emit()
	}

	phrase := rule.TerminalNames(grammar)
	if len(phrase) < 2 {
		return emit()
	}

	inspectMatch(current, phrase, metrics)

	matchedPath := strings.Join(phrase, "/")
	meta := []*data.StringEntry{
		{Key: "token_path", Value: matchedPath},
		{Key: "token_depth", Value: strconv.Itoa(len(phrase))},
	}

	action := current.actions[matchedPath]
	if action == "" {
		return emit(meta...)
	}

	meta = append(meta,
		&data.StringEntry{Key: "decision_action", Value: action},
		&data.StringEntry{Key: "decision_reason", Value: fmt.Sprintf(
			"path=%s %s", matchedPath, training.act(prior.Label, action, matchedPath),
		)},
	)

	return emit(meta...)
}

/*
ReloadKeys loads the published action tree from S3 and swaps it in whole.
*/
func (training *Training) ReloadKeys(ctx context.Context) error {
	loaded := &policy{
		keys:    make([]string, 0),
		actions: make(map[string]string),
	}

	if training.client == nil {
		training.policy.Store(loaded)
		return nil
	}

	for measurement := range training.client.Get("policy/") {
		if measurement == nil {
			continue
		}

		if measurement.Source != "policy" {
			continue
		}

		action := ""
		if actEntry := data.Pull(measurement.Meta("action")); actEntry != nil {
			action = actEntry.Value
		}

		pathKey := measurement.Label
		if pathKey == "" {
			continue
		}

		if existing, found := loaded.actions[pathKey]; found && existing != action {
			action = ""
		}

		loaded.actions[pathKey] = action
	}

	for dir, action := range loaded.actions {
		var file string

		if action == "enter" {
			file = actionEnter
		}

		if action == "exit" {
			file = actionExit
		}

		if action == "noop" {
			file = actionNoop
		}

		if file == "" {
			continue
		}

		loaded.keys = append(loaded.keys, dir+"/"+file)
	}

	slices.Sort(loaded.keys)
	training.policy.Store(loaded)

	errnie.Info(fmt.Sprintf(
		"[training] loaded %d stored paths",
		len(loaded.keys),
	))

	return nil
}

/*
resetPaperAndReload resets the Kraken paper account to its default state ($200, 0.8% fee)
and reloads the published action tree.
*/
func (training *Training) resetPaperAndReload() {
	errnie.Info("[training] paper account exhausted or below minimum order; resetting paper account and reloading prefixes...")

	if err := training.desk.ResetPaper(); err != nil {
		training.Error(errnie.Err(
			errnie.IO,
			"[training] failed to reset paper account",
			err,
		))

		return
	}

	if err := training.ReloadKeys(training.Context()); err != nil {
		training.Error(errnie.Err(
			errnie.IO,
			"[training] failed to reload stored prefix keys after paper reset",
			err,
		))

		return
	}

	errnie.Info(fmt.Sprintf(
		"[training] paper account reset to default ($200, 0.8%% fee); reloaded %d prefix paths into memory",
		len(training.policy.Load().keys),
	))
}

/*
act takes the matched action through the Desk when the position state admits
it, and reports what happened. A symbol holds at most one position: entering
needs it flat, exiting needs it held.

Entries are opened sized against configured allocation fraction and available cash.
Exits sell everything currently open.
*/
func (training *Training) act(symbol, action string, path ...string) string {
	switch action {
	case "enter":
		if training.desk.Holds(symbol) {
			training.paths[symbol] = nil
			return "holding: enter ignored"
		}

		cash := training.desk.Cash(system.Cfg.Market.QuoteCurrency)

		if cash == nil || cash.Sign() <= 0 {
			training.resetPaperAndReload()
			cash = training.desk.Cash(system.Cfg.Market.QuoteCurrency)
		}

		if cash == nil || cash.Sign() <= 0 {
			training.Error(errnie.Err(
				errnie.NotAcceptable,
				"[training] cash is unavailable for quote currency "+system.Cfg.Market.QuoteCurrency,
				nil,
			))

			training.paths[symbol] = nil
			return "enter failed: no cash"
		}

		allocated := cash.SetScale(4).Mul(system.Cfg.Risk.MaxFraction)
		entryCost, err := training.price.AllocateEntry(symbol, allocated)

		if err != nil {
			training.resetPaperAndReload()
			cash = training.desk.Cash(system.Cfg.Market.QuoteCurrency)

			if cash != nil && cash.Sign() > 0 {
				allocated = cash.SetScale(4).Mul(system.Cfg.Risk.MaxFraction)
				entryCost, err = training.price.AllocateEntry(symbol, allocated)
			}
		}

		if err != nil {
			training.Error(errnie.Err(
				errnie.NotAcceptable,
				"[training] unable to calculate quantity for "+symbol,
				err,
			))

			training.paths[symbol] = nil
			return "enter failed: no quantity"
		}

		if err := training.desk.Enter(symbol, entryCost.Quantity, path...); err != nil {
			training.Error(errnie.Err(errnie.IO, "[training] enter failed for "+symbol, err))
			training.paths[symbol] = nil
			return "enter failed: " + err.Error()
		}

		training.paths[symbol] = nil
		return "enter submitted"
	case "exit":
		if !training.desk.Holds(symbol) {
			training.paths[symbol] = nil
			return "flat: exit ignored"
		}

		if err := training.desk.Exit(symbol); err != nil {
			training.Error(errnie.Err(errnie.IO, "[training] exit failed for "+symbol, err))
			training.paths[symbol] = nil
			return "exit failed: " + err.Error()
		}

		training.paths[symbol] = nil
		return "exit submitted"
	case "noop":
		training.paths[symbol] = nil
		return "noop: matched unpromising precursor"
	}

	return "ignored: unknown action " + action
}
