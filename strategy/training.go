package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
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
	coders   map[string]*learning.PredictiveCoder
	policy   atomic.Pointer[policy]
	grammars map[string]*Grammar
	lastSym  map[string]string
	paths    map[string][]string
	last     map[string]*data.Measurement
	step     int64
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

func newPredictiveCoder() *learning.PredictiveCoder {
	arch := []int{15, 60, 30, 15}
	pace := learning.NewPace(0.05, 0.005, 0.150, 0.1, 0.2, 256)
	target := learning.NewDirectionalTarget(0)
	return learning.NewPredictiveCoder(
		arch, 8, target, pace, 0.05, true, learning.ReadoutAll,
	).(*learning.PredictiveCoder)
}

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
		coders:   make(map[string]*learning.PredictiveCoder),
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
	pol := training.policy.Load()
	if pol == nil {
		return ui.CognitionTreeExport{
			Branches: []ui.TrieBranchJSON{},
		}
	}

	keys := pol.keys
	branches := make([]ui.TrieBranchJSON, 0, len(keys))
	
	for _, key := range keys {
		clean := strings.TrimSuffix(key, ".bin")
		parts := strings.Split(clean, "/")
		
		if len(parts) < 2 {
			continue
		}
		
		rawAction := strings.ToUpper(parts[len(parts)-1])
		policy := "NOOP"
		
		if rawAction == "ENTER" || rawAction == "EXIT" {
			policy = rawAction
		}
		
		hash := strings.Join(parts[:len(parts)-1], "/")
		depth := len(parts) - 1
		meanEdge := 0.0

		switch policy {
		case "ENTER":
			meanEdge = 0.05
		case "EXIT":
			meanEdge = -0.02
		}
		
		branches = append(branches, ui.TrieBranchJSON{
			Hash:       hash,
			Depth:      depth,
			Visits:     1,
			MeanEdge:   meanEdge,
			Confidence: 95.0,
			Policy:     policy,
		})
	}

	return ui.CognitionTreeExport{
		Keys:     keys,
		Branches: branches,
	}
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

func (training *Training) CognitionGrammar(symbol string) GrammarExport {
	if symbol == "" {
		symbol = "BTC/USD"
	}

	grammar := training.grammars[symbol]
	
	if grammar == nil {
		return GrammarExport{Symbol: symbol, Rules: []GrammarRuleExport{}}
	}

	current := training.policy.Load()
	rules := grammar.Rules()
	exports := make([]GrammarRuleExport, 0, len(rules))

	for _, rule := range rules {
		terms := rule.TerminalNames(grammar)
		termPath := strings.Join(terms, "/")
		action := ""
		if current != nil && current.actions != nil {
			if act, found := current.actions[termPath]; found {
				action = act
			}
		}

		syms := make([]string, 0)
		for currentSymbol := rule.guard.next; currentSymbol != rule.guard && currentSymbol != nil; currentSymbol = currentSymbol.next {
			if currentSymbol.rule != nil {
				syms = append(syms, fmt.Sprintf("M%d", currentSymbol.rule.id))
			} else {
				syms = append(syms, grammar.Name(currentSymbol.value))
			}
		}

		exports = append(exports, GrammarRuleExport{
			ID:        rule.ID(),
			Name:      fmt.Sprintf("M%d", rule.ID()),
			Count:     rule.Count(),
			Depth:     rule.Depth(),
			Length:    rule.Length(),
			Symbols:   syms,
			Terminals: terms,
			Action:    action,
		})
	}

	startSyms := make([]string, 0)
	
	if grammar.startRule != nil {
		for s := grammar.startRule.guard.next; s != grammar.startRule.guard && s != nil; s = s.next {
			if s.rule != nil {
				startSyms = append(startSyms, fmt.Sprintf("M%d", s.rule.id))
			} else {
				startSyms = append(startSyms, grammar.Name(s.value))
			}
		}
	}

	return GrammarExport{
		Symbol: symbol,
		Rules:  exports,
		Start:  startSyms,
	}
}

/*
Step receives the live market join, scores its regions through the grid,
advances the predictive coder over the 15 regions, and gates emission of
active regions into Sequitur via the dynamic midpoint.
*/
func (training *Training) Step(prior *data.Measurement) *data.Measurement {
	regions := training.grid.Observe(prior)
	metrics := make(map[string]float64, len(regions)*2+2)

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

	coder, exists := training.coders[prior.Label]
	if !exists {
		coder = newPredictiveCoder()
		training.coders[prior.Label] = coder
	}

	grammar, exists := training.grammars[prior.Label]
	if !exists {
		grammar = NewGrammar()
		training.grammars[prior.Label] = grammar
	}

	// 1. Vector of 15 region brightnesses
	regionBrightness := make([]float64, 15)
	for _, r := range regions {
		if int(r.Key) < 15 {
			regionBrightness[r.Key] = r.Brightness
		}
	}

	// 2. Resonance PredictiveCoder settles over the 15 regions
	training.step++
	inValue := data.NewValue([2][]float64{
		regionBrightness,
		{0.0, 0.0, float64(training.step), float64(prior.At.UnixNano()) / 1e9},
	})

	var out [12][]float64
	for val := range coder.Next(inValue.Next(nil)) {
		out = *(*[12][]float64)(val)
	}

	surprise := 0.0
	energy := 0.0
	if len(out[0]) > 1 {
		energy = out[0][1]
	}
	if len(out[0]) > 5 {
		surprise = out[0][5]
	}
	metrics["resonance_surprise"] = surprise
	metrics["resonance_energy"] = energy

	// 3. Dynamic Midpoint Activation Gate:
	active := make([]*nmstore.Region, 0)
	for _, r := range regions {
		if r.Brightness > 0 {
			active = append(active, r)
		}
	}

	if len(active) == 0 {
		return emit()
	}

	maxB := active[0].Brightness
	minB := active[len(active)-1].Brightness
	midB := (maxB + minB) / 2.0

	var firedTokens []string
	for _, r := range active {
		if r.Brightness >= midB {
			firedTokens = append(firedTokens, fmt.Sprintf("R%02d", r.Key))
		}
	}

	if len(firedTokens) == 0 {
		return emit()
	}

	chordKey := strings.Join(firedTokens, "/")
	if chordKey == training.lastSym[prior.Label] {
		return emit()
	}
	training.lastSym[prior.Label] = chordKey

	tokens := training.paths[prior.Label]
	var deepestRule *Rule

	for _, token := range firedTokens {
		tokens = append(tokens, token)
		r := grammar.Append(token)
		if r != nil {
			deepestRule = r
		}
	}

	horizon := 16
	if system.Cfg != nil && system.Cfg.Learning.PrecursorHorizon > 0 {
		horizon = system.Cfg.Learning.PrecursorHorizon
	}
	if len(tokens) > horizon {
		tokens = tokens[len(tokens)-horizon:]
	}
	training.paths[prior.Label] = tokens

	var phrase []string
	if deepestRule != nil {
		phrase = deepestRule.TerminalNames(grammar)
	}

	tokensToMatch := tokens
	if len(phrase) >= 2 {
		tokensToMatch = phrase
	}

	inspectMatch(current, tokensToMatch, metrics, prior.Label)

	matchedPath := strings.Join(tokensToMatch, "/")
	action := ""

	minConf := 2
	if system.Cfg != nil && system.Cfg.Learning.MinimumPathConfidence >= 2 {
		minConf = system.Cfg.Learning.MinimumPathConfidence
	}

	for d := len(tokensToMatch); d >= minConf; d-- {
		candidate := strings.Join(tokensToMatch[len(tokensToMatch)-d:], "/")
		if act, found := current.actions[candidate]; found && act != "" {
			action = act
			matchedPath = candidate
			break
		}
		if act, found := current.actions[prior.Label+"/"+candidate]; found && act != "" {
			action = act
			matchedPath = candidate
			break
		}
	}

	grammarJSON, _ := json.Marshal(training.CognitionGrammar(prior.Label))

	meta := []*data.StringEntry{
		{Key: "token_path", Value: matchedPath},
		{Key: "token_depth", Value: strconv.Itoa(len(tokensToMatch))},
		{Key: "sequitur_grammar", Value: string(grammarJSON)},
		{Key: "resonance_surprise", Value: fmt.Sprintf("%.6f", surprise)},
		{Key: "active_tokens", Value: chordKey},
	}

	if action == "" {
		return emit(meta...)
	}

	reason := training.act(prior.Label, action, matchedPath)

	meta = append(meta,
		&data.StringEntry{Key: "decision_action", Value: action},
		&data.StringEntry{Key: "action", Value: action},
		&data.StringEntry{Key: "decision_reason", Value: fmt.Sprintf("path=%s %s", matchedPath, reason)},
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

	s3Keys := training.client.ListKeys("policy/current/")
	if len(s3Keys) == 0 {
		s3Keys = training.client.ListKeys("policy/")
	}

	for _, key := range s3Keys {
		if !strings.HasPrefix(key, "policy/") || !strings.HasSuffix(key, ".bin") {
			continue
		}

		if strings.HasPrefix(key, "policy/archive/") {
			continue
		}

		sub := strings.TrimPrefix(key, "policy/")
		sub = strings.TrimSuffix(sub, ".bin")
		parts := strings.Split(sub, "/")

		if len(parts) < 2 {
			continue
		}

		action := parts[len(parts)-1]
		if action != "enter" && action != "exit" && action != "noop" {
			continue
		}

		firstR := -1
		for i := 0; i < len(parts)-1; i++ {
			if strings.HasPrefix(parts[i], "R") {
				firstR = i
				break
			}
		}

		if firstR < 0 {
			continue
		}

		path := strings.Join(parts[firstR:len(parts)-1], "/")
		if path == "" {
			continue
		}

		// Map path globally
		if existing, found := loaded.actions[path]; found && existing != action {
			action = ""
		}

		loaded.actions[path] = action
		loaded.keys = append(loaded.keys, path+"/"+parts[len(parts)-1]+".bin")

		// Also preserve symbol-scoped key if key has a symbol prefix
		symbol := parts[0]
		if firstR > 0 {
			symEnd := firstR
			if firstR > 1 {
				if _, err := strconv.ParseInt(parts[firstR-1], 10, 64); err == nil {
					symEnd = firstR - 1
				}
			}
			symbol = strings.Join(parts[:symEnd], "/")
		}

		if symbol != "" && symbol != "current" {
			loaded.actions[symbol+"/"+path] = action
		}
	}

	if len(loaded.actions) == 0 {
		for measurement := range training.client.Get("policy/") {
			if measurement == nil || measurement.Source != "policy" {
				continue
			}

			action := ""
			if actEntry := data.Pull(measurement.Meta("action")); actEntry != nil {
				action = actEntry.Value
			}

			pathKey := ""
			if pathEntry := data.Pull(measurement.Meta("path")); pathEntry != nil {
				pathKey = pathEntry.Value
			}

			if pathKey == "" || measurement.Label == "" {
				continue
			}

			actionKey := measurement.Label + "/" + pathKey
			if existing, found := loaded.actions[actionKey]; found && existing != action {
				action = ""
			}

			loaded.actions[actionKey] = action

			var file string
			switch action {
			case "enter":
				file = actionEnter
			case "exit":
				file = actionExit
			case "noop":
				file = actionNoop
			}

			if file != "" {
				loaded.keys = append(loaded.keys, measurement.Label+"/"+pathKey+"/"+file)
			}
		}
	}

	slices.Sort(loaded.keys)
	loaded.keys = slices.Compact(loaded.keys)
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
