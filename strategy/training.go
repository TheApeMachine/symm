package strategy

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
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
	price        *broker.Price
	desk         *broker.Desk
	client       *store.Client
	grid         *nmstore.Grid
	policy       atomic.Pointer[policy]
	hub          io.Writer
	symbols      map[string]*symbolGrammar
	published    map[string]string
	archived     map[string]struct{}
	virtuals     virtualBook
	bars         stepBars
	virtualShown map[string]struct{}
}

/*
symbolGrammar is one instrument's Sequitur and the token path it has built.
The grid is shared; the grammar is not.
*/
type symbolGrammar struct {
	grammar    *Grammar
	lastChord  string
	lastRule   *Rule
	tokenPath  []string
	sentence   []string
	regime     regime
	held       int
	brightness map[int]float64
	strength   float64
	threshold  float64
}

/*
cutSentence ends the tokens the desk is matching. The chord that just
matched stays, so the same chord does not fire again on the next tick.
*/
func (state *symbolGrammar) cutSentence() {
	if state == nil {
		return
	}

	state.tokenPath = nil
	state.sentence = nil
	state.lastRule = nil
}

/*
breakSentence ends the regime. The next chord starts a new sentence, even
when it repeats the chord that just broke.
*/
func (state *symbolGrammar) breakSentence() {
	state.cutSentence()

	if state == nil {
		return
	}

	state.lastChord = ""
}

/*
SetHub is where live grammar frames go. The dashboard Sequitur tab reads them.
*/
func (training *Training) SetHub(hub io.Writer) {
	if training == nil {
		return
	}

	training.hub = hub
	training.replayClosed()
}

/*
writeFrame copies value to the hub. A hub that can Retain keeps the latest
frame under key and replays it when a dashboard connects.
*/
func (training *Training) writeFrame(key string, value *data.Value) error {
	if training == nil || training.hub == nil || value == nil {
		return nil
	}

	writer := training.hub

	if key != "" {
		if kept, ok := training.hub.(interface{ Retain(string) io.Writer }); ok {
			writer = kept.Retain(key)
		}
	}

	_, err := io.Copy(writer, value)
	return err
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
)

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
	client *store.Client,
) *Training {
	training := &Training{
		System: runtime.NewSystem(ctx, "training", price),
		price:  price,
		desk:   desk,
		client: client,
		grid:   nmstore.NewGrid(ctx),
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

	loadErr := training.ReloadKeys(ctx)
	training.restoreOpenPositions()
	training.loadBars()
	training.loadVirtuals()

	if loadErr != nil {
		training.Error(errnie.Err(
			errnie.IO, "[training] unable to load the published action tree", loadErr,
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

func (training *Training) symbolState(symbol string) *symbolGrammar {
	if training.symbols == nil {
		training.symbols = map[string]*symbolGrammar{}
	}

	state, ok := training.symbols[symbol]

	if !ok {
		state = &symbolGrammar{grammar: NewGrammar()}
		training.symbols[symbol] = state
	}

	if state.grammar == nil {
		state.grammar = NewGrammar()
	}

	return state
}

func (training *Training) CognitionGrammar(symbol string) GrammarExport {
	if symbol == "" || training == nil || training.symbols == nil || training.symbols[symbol] == nil {
		return GrammarExport{Symbol: symbol, Rules: []GrammarRuleExport{}}
	}

	grammar := training.symbols[symbol].grammar

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
Step receives the live market join, scores its regions through the grid, and
appends the regions at or above the brightness midpoint to Sequitur.
*/
func (training *Training) Step(value *data.Value) *data.Value {
	if value == nil {
		return nil
	}

	defer training.publishHeld(value)

	if _, err := training.grid.Observe(value); err != nil {
		training.Error(err)
		return value
	}

	bySymbol := training.grid.SymbolRegions()
	if len(bySymbol) == 0 {
		return value
	}

	names := make([]string, 0, len(bySymbol))
	for symbol := range bySymbol {
		names = append(names, symbol)
	}
	slices.Sort(names)

	for _, symbol := range names {
		training.stepSymbol(value, symbol, bySymbol[symbol])
	}

	return value
}

/*
stepSymbol turns one symbol's lit regions into its own chord. A new chord
is appended to that symbol's grammar, matched against the stored paths, and
published. A repeated chord is held. The sentence ends when two regimes
describe this symbol's metric energy in a shorter code than one regime;
Sequitur keeps the rules it has already learned. The hub replays the last
frame when a dashboard connects.
*/
func (training *Training) stepSymbol(value *data.Value, symbol string, regions []*nmstore.Region) {
	training.markSymbolVirtuals(symbol)

	current := training.policy.Load()
	if current == nil {
		current = &policy{actions: map[string]string{}}
	}

	state := training.symbolState(symbol)

	if state.regime.advance(training.grid.Energy(symbol)) {
		state.breakSentence()
	}

	grammar := state.grammar
	state.brightness = make(map[int]float64, len(regions))

	for _, region := range regions {
		if region == nil {
			continue
		}

		state.brightness[int(region.Key)] = region.Brightness
	}

	active := make([]*nmstore.Region, 0)
	for _, region := range regions {
		if region != nil && region.Brightness > 0 {
			active = append(active, region)
		}
	}

	if len(active) == 0 {
		return
	}

	midB := (active[0].Brightness + active[len(active)-1].Brightness) / 2.0
	state.strength = active[0].Brightness
	state.threshold = midB

	prefix := append([]string{}, state.tokenPath...)
	firedTokens := make([]string, 0, len(active))

	for _, region := range active {
		token := fmt.Sprintf("R%02d", region.Key)

		if !training.admitToken(prefix, token, region.Brightness, midB) {
			continue
		}

		firedTokens = append(firedTokens, token)
		prefix = append(prefix, token)
	}

	if len(firedTokens) == 0 {
		return
	}

	chordKey := strings.Join(firedTokens, "/")
	newChord := chordKey != state.lastChord

	if newChord {
		state.held = 0
		state.lastChord = chordKey

		tokens := state.tokenPath

		for _, token := range firedTokens {
			tokens = append(tokens, token)
			state.sentence = append(state.sentence, token)
			if rule := grammar.Append(token); rule != nil {
				state.lastRule = rule
			}
		}

		horizon := 16
		if system.Cfg != nil && system.Cfg.Learning.PrecursorHorizon > 0 {
			horizon = system.Cfg.Learning.PrecursorHorizon
		}
		if len(tokens) > horizon {
			tokens = tokens[len(tokens)-horizon:]
		}
		state.tokenPath = tokens
	} else {
		state.held++
		return
	}

	var phrase []string
	if state.lastRule != nil {
		phrase = state.lastRule.TerminalNames(grammar)
	}

	if !phraseInSentence(phrase, state.sentence) {
		phrase = nil
	}

	matchedPath := ""
	action := ""

	minConf := 2
	if system.Cfg != nil && system.Cfg.Learning.MinimumPathConfidence >= 2 {
		minConf = system.Cfg.Learning.MinimumPathConfidence
	}

	// The stored key is the rule Sequitur just completed. A raw token window
	// and a shorter suffix are not substitutes for that rule.
	if len(phrase) >= minConf {
		matchedPath = strings.Join(phrase, "/")
		training.noteSteps(matchedPath, tokenBrightness(state, phrase))
		training.openVirtual(symbol, phrase, tokenBrightness(state, phrase))

		if act, found := current.actions[symbol+"/"+matchedPath]; found && act != "" {
			action = act
		} else if act, found := current.actions[matchedPath]; found && act != "" {
			action = act
		}
	}

	outcome := ""

	if action != "" {
		outcome = training.act(state, symbol, action, matchedPath)
	}

	tokens := firedTokens
	if len(phrase) >= minConf {
		tokens = phrase
	}

	depth := len(tokens)
	if action != "" && matchedPath != "" {
		depth = len(phrase)
	}

	if err := training.publishCognitionMatch(value, symbol, tokens, matchedPath, depth, action); err != nil {
		training.Error(err)
	}

	if err := training.publishSequitur(value, symbol, active, midB, firedTokens, state.held, matchedPath, state.lastRule, action, outcome); err != nil {
		training.Error(err)
	}
}

func tokenBrightness(state *symbolGrammar, phrase []string) []float64 {
	if state == nil {
		return nil
	}

	out := make([]float64, len(phrase))

	for i, token := range phrase {
		if len(token) != 3 || token[0] != 'R' {
			continue
		}

		key, err := strconv.Atoi(token[1:])

		if err != nil {
			continue
		}

		out[i] = state.brightness[key]
	}

	return out
}

/*
scopedPolicySymbols lists instruments to try when matching symbol-prefixed
policy paths on an aggregate tick: dashboard focus first, then symbols that
traded in this frame.
*/
func (training *Training) scopedPolicySymbols(value *data.Value) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 4)

	if focus := types.Focus(); focus != "" {
		out = append(out, focus)
		seen[focus] = struct{}{}
	}

	traded, err := broker.TradedSymbols(value)

	if err != nil {
		errnie.Error(err)
		return out
	}

	for _, sym := range traded {
		if _, ok := seen[sym]; ok {
			continue
		}

		seen[sym] = struct{}{}
		out = append(out, sym)
	}

	return out
}

/*
tradeSymbol picks the desk instrument for a globally matched path: an explicit
policy symbol wins; otherwise focus, a sole traded symbol, or the first trade
in the frame.
*/
func (training *Training) tradeSymbol(value *data.Value, scoped []string, policySymbol string) string {
	if policySymbol != "" {
		return policySymbol
	}

	if focus := types.Focus(); focus != "" {
		return focus
	}

	traded, err := broker.TradedSymbols(value)

	if err != nil {
		errnie.Error(err)
		return ""
	}

	if len(traded) == 1 {
		return traded[0]
	}

	if len(scoped) > 0 {
		return scoped[0]
	}

	if len(traded) > 0 {
		return traded[0]
	}

	return ""
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
		if action != "enter" && action != "exit" {
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
		for val, err := range store.Values(training.client, "policy/") {
			if err != nil {
				return errnie.Err(errnie.IO, "[training] reading policy", err)
			}

			if val.Origin != "policy" {
				continue
			}

			action := val.Role
			symbol := val.Scope
			pathKey := ""

			for _, idx := range val.Index {
				for part := range strings.SplitSeq(idx, "/") {
					if after, ok := strings.CutPrefix(part, "path:"); ok {
						pathKey = after
						break
					}
				}
				if pathKey != "" {
					break
				}
			}

			if pathKey == "" || symbol == "" {
				continue
			}

			if action != "enter" && action != "exit" {
				continue
			}

			actionKey := symbol + "/" + pathKey
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
			}

			if file != "" {
				loaded.keys = append(loaded.keys, symbol+"/"+pathKey+"/"+file)
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
resetPaperAndReload resets the Kraken paper account to its default state ($200, 0.8% fee),
drops the open and closed lots that belonged to the spent account, and reloads the
published action tree.
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

	training.dropWalletLots()

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
func (training *Training) act(state *symbolGrammar, symbol, action string, path ...string) string {
	switch action {
	case "enter":
		if training.desk.Holds(symbol) {
			state.cutSentence()
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

			state.cutSentence()
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

			state.cutSentence()
			return "enter failed: no quantity"
		}

		if err := training.desk.Enter(symbol, entryCost.Quantity, path...); err != nil {
			training.Error(errnie.Err(errnie.IO, "[training] enter failed for "+symbol, err))
			state.cutSentence()
			return "enter failed: " + err.Error()
		}

		state.cutSentence()
		return "enter submitted"
	case "exit":
		if !training.desk.Holds(symbol) {
			state.cutSentence()
			return "flat: exit ignored"
		}

		if err := training.desk.Exit(symbol); err != nil {
			training.Error(errnie.Err(errnie.IO, "[training] exit failed for "+symbol, err))
			state.cutSentence()
			return "exit failed: " + err.Error()
		}

		state.cutSentence()
		return "exit submitted"
	}

	return "ignored: unknown action " + action
}
