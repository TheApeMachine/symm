package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

/*
Training paper trades the live market against the action tree the Trainer
published: every frame becomes a region token, the token advances the
symbol's token path through the published Tokenizer, and once the path is
Depth tokens long and moves onto a stored path whose actions all agree, that
action is taken through the Desk.
*/
type Training struct {
	*runtime.System
	price   *broker.Price
	desk    *broker.Desk
	catalog *tables.Catalog
	grid    *store.Grid
	policy  atomic.Pointer[policy]
	seen    Tokenizer
	paths   map[string][]string
	last    map[string]*data.Measurement
}

/*
policy is one loaded generation of the published action tree: its Tokenizer
(nil while none is published, which means no trading), its keys, and the one
action each stored path agrees on ("" where its actions disagree). It is
swapped whole, so Step never pairs a Tokenizer with another generation's
paths.
*/
type policy struct {
	tokenizer *Tokenizer
	keys      []string
	actions   map[string]string
}

/*
Stored path actions: the final segment of every key.
*/
const (
	actionEnter = "enter.json"
	actionExit  = "exit.json"
	actionNoop  = "noop.json"
)

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
	catalog *tables.Catalog,
) *Training {
	training := &Training{
		System:  runtime.NewSystem(ctx, "training", price),
		price:   price,
		desk:    desk,
		catalog: catalog,
		grid:    store.NewGrid(ctx),
		paths:   make(map[string][]string),
		last:    make(map[string]*data.Measurement),
	}

	training.policy.Store(&policy{})

	if err := errnie.Require(map[string]any{
		"price":   price,
		"desk":    desk,
		"catalog": catalog,
	}); err != nil {
		training.Error(errnie.Err(
			errnie.Validation, "[training] cannot paper trade without price, desk and catalog", err,
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
and advances the symbol's token path, acting when the path lands on a
stored path with one agreed action.
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

	if current.tokenizer == nil || regions == nil {
		return emit()
	}

	// Paths built under other Tokenizer parameters are not paths of this one.
	if *current.tokenizer != training.seen {
		training.seen = *current.tokenizer
		training.paths = make(map[string][]string)
	}

	active, strength := current.tokenizer.Active(regions)
	tokens := training.paths[prior.Label]
	moved := false

	for _, tok := range active {
		var advanced bool
		tokens, advanced = current.tokenizer.Advance(tokens, tok, strength)

		if advanced {
			moved = true
		}
	}

	training.paths[prior.Label] = tokens
	joined := strings.Join(tokens, "/")
	inspectMatch(current, tokens, strength, metrics)

	meta := []*data.StringEntry{
		{Key: "token_path", Value: joined},
		{Key: "token_depth", Value: strconv.Itoa(len(tokens))},
	}

	if !moved {
		return emit(meta...)
	}

	matchedPath, action := MatchSuffix(tokens, current.actions)

	if action == "" {
		return emit(meta...)
	}

	meta = append(meta,
		&data.StringEntry{Key: "decision_action", Value: action},
		&data.StringEntry{Key: "decision_reason", Value: fmt.Sprintf("path=%s %s", matchedPath, training.act(prior.Label, action, matchedPath))},
	)

	return emit(meta...)
}

/*
ReloadKeys loads the published action tree from S3 and swaps it in whole.
Without a published Tokenizer the loaded policy holds the keys but never
trades.
*/
func (training *Training) ReloadKeys(ctx context.Context) error {
	loaded := &policy{
		keys:    make([]string, 0),
		actions: make(map[string]string),
	}

	for key, err := range training.catalog.ListBlobs(ctx, "") {
		if errors.Is(err, tables.ErrBlobStorageUnconfigured) {
			training.policy.Store(loaded)
			return nil
		}

		if err != nil {
			return errnie.Error(err)
		}

		dir, file := path.Split(key)

		if file != actionEnter && file != actionExit && file != actionNoop {
			continue
		}

		dir = strings.TrimSuffix(dir, "/")
		action := strings.TrimSuffix(file, ".json")

		if existing, found := loaded.actions[dir]; found && existing != action {
			action = ""
		}

		loaded.actions[dir] = action
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
	body, err := training.catalog.GetBlob(ctx, tokenizerKey)

	if errors.Is(err, tables.ErrBlobMissing) {
		training.policy.Store(loaded)

		errnie.Info(fmt.Sprintf(
			"[training] %d stored token paths but no %s yet: not trading",
			len(loaded.keys), tokenizerKey,
		))

		return nil
	}

	if err != nil {
		return errnie.Error(err)
	}

	tokenizer := &Tokenizer{}

	if err := json.Unmarshal(body, tokenizer); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "[training] unreadable "+tokenizerKey, err))
	}

	if err := tokenizer.validate(); err != nil {
		return errnie.Error(err)
	}

	loaded.tokenizer = tokenizer
	training.policy.Store(loaded)

	errnie.Info(fmt.Sprintf(
		"[training] loaded %d stored token paths (depth=%d regions=%d threshold=%g)",
		len(loaded.keys), tokenizer.Depth, tokenizer.Regions, tokenizer.Threshold,
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

	if err := training.ReloadKeys(training.Context()); err != nil && !errors.Is(err, tables.ErrBlobStorageUnconfigured) {
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
