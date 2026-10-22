package strategy

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/types"
)

type (
	Token  = tokenizer.Token
	Tokens = tokenizer.Tokens
)

/*
key is a token's region address: its first path element (R08, or a chord
production's head). String() already renders the full path.
*/
func key(token *Token) string {
	if len(token.Path) == 0 {
		return ""
	}

	return token.Path[0]
}

/*
Training paper trades the live market against the action tree the Trainer
published: every frame becomes a region token, the token advances the
symbol's grammar through Sequitur, and when an induced rule matches a stored
path with an agreed action, that action is taken through the Desk.
*/
type Training struct {
	*runtime.System
	price      *broker.Price
	desk       *broker.Desk
	client     *store.Client
	grid       *nmstore.Grid
	prefixes   [][]*Token
	actions    map[string]string
	model      *Model
	tokenizers *sync.Map
	policy     *sync.Map
	hub        io.Writer
}

func NewTraining(
	ctx context.Context,
	price *broker.Price,
	desk *broker.Desk,
	client *store.Client,
	hub io.Writer,
) *Training {
	training := &Training{
		System:     runtime.NewSystem(ctx, "training", price),
		price:      price,
		desk:       desk,
		client:     client,
		grid:       nmstore.NewGrid(ctx),
		prefixes:   make([][]*Token, 0),
		model:      NewModel(ctx, desk, client),
		tokenizers: &sync.Map{},
		hub:        hub,
		policy:     &sync.Map{},
		actions:    map[string]string{},
	}

	training.reloadPrefixes()
	training.Transition(runtime.INIT)

	// Closed virtuals settle through this training's model so path scores
	// (Wins/Promoted) land wherever the virtual opened.
	desk.BindSettler(training.model)

	return training
}

func (training *Training) Step(value *data.Value) *data.Value {
	if value == nil {
		return nil
	}

	grouped := make(map[string]bool)

	for _, idx := range value.Index {
		rest := idx
		if value.Prefix != "" && strings.HasPrefix(idx, value.Prefix+"/") {
			rest = strings.TrimPrefix(idx, value.Prefix+"/")
		}
		parts := strings.Split(rest, "/")
		// A signal layer key is source/symbol/label..., e.g. "cvd/BTC/USD/midpoint".
		// Only keys from known signal sources with a fully-qualified symbol
		// (base/quote) are tradable; skip spot:trade, train/*, and anything else.
		if len(parts) < 3 {
			continue
		}

		source := parts[0]
		isSignal := slices.Contains(types.SignalSourceStrings, source)

		if !isSignal {
			continue
		}

		symbol := parts[1] + "/" + parts[2]
		grouped[symbol] = true
	}

	for symbol := range grouped {
		found, _ := training.policy.LoadOrStore(symbol, []*Token{})
		tokens := found.([]*Token)
		regions := make([]*Token, 0)
		observed, err := training.grid.Observe(value)

		if err != nil {
			training.Error(err)
			continue
		}

		for _, region := range observed {
			regions = append(regions, tokenizer.NewToken(
				[]string{fmt.Sprintf("R%02d", region.Key)},
				region.Brightness,
				region.Brightness,
			))
		}

		seq, _ := training.tokenizers.LoadOrStore(symbol, tokenizer.NewSequitur())
		tok := seq.(*tokenizer.Sequitur)

		admit := training.match(tokens, regions)

		for _, region := range training.belowEnergy(tokens, regions) {
			value.Append(
				[]float64{region.Brightness},
				"train/reject/"+symbol+"/"+key(region),
			)
		}

		rule, ok := tok.Analyze(admit)

		if ok {
			path := strings.Join(tok.TerminalNames(rule), "/")
			training.act(symbol, Tokens{tokenizer.NewToken([]string{path}, 0, 0)})
			training.policy.Store(symbol, []*Token{})
			training.tokenizers.Store(symbol, tokenizer.NewSequitur())
			continue
		}

		if len(regions) > 0 && len(admit) == 0 {
			training.policy.Store(symbol, []*Token{})
			continue
		}

		accepted := admitted(regions, admit)
		tokens = append(tokens, accepted...)
		training.policy.Store(symbol, tokens)
		training.act(symbol, tokens)

		for _, region := range accepted {
			value.Append(
				[]float64{region.Brightness},
				"train/regions/"+symbol+"/"+key(region),
			)
		}

		if len(tokens) > 0 {
			keys := make([]float64, len(tokens))
			labels := make([]string, len(tokens))

			for i, t := range tokens {
				keys[i] = t.Brightness
				labels[i] = key(t)
			}

			value.Append(
				keys,
				"train/regions/"+symbol+"/"+strings.Join(labels, "/"),
			)
		}
	}

	return value
}

/*
match checks if the region has enough energy to overcome the
threshold needed to allow matching the tokens.
*/
func (training *Training) match(sentence, regions []*Token) []string {
	if training.model != nil && training.model.Status() == runtime.READY {
		return training.model.Match(sentence, regions)
	}

	keys := make([]string, 0, len(regions))

	for i, region := range regions {
		step := len(sentence) + i

		if len(training.prefixes) == 0 {
			keys = append(keys, key(region))
			continue
		}

		if step >= len(training.prefixes) {
			continue
		}

		for _, gate := range training.prefixes[step] {
			if key(region) == key(gate) && region.Brightness >= gate.Threshold {
				keys = append(keys, key(region))
				break
			}
		}
	}

	return keys
}

/*
belowEnergy is a region whose key is the next policy token and whose
brightness is under that token's energy gate. Those regions are not
admitted, and the caller publishes them so the trie can show the refusal.
*/
func (training *Training) belowEnergy(sentence, regions []*Token) []*Token {
	if training.model != nil && training.model.Status() == runtime.READY {
		return training.model.Below(sentence, regions)
	}

	if len(training.prefixes) == 0 {
		return nil
	}

	out := make([]*Token, 0)

	for i, region := range regions {
		step := len(sentence) + i

		if step >= len(training.prefixes) {
			continue
		}

		for _, gate := range training.prefixes[step] {
			if key(region) == key(gate) && region.Brightness < gate.Threshold {
				out = append(out, region)
				break
			}
		}
	}

	return out
}

func (training *Training) act(symbol string, tokens Tokens) {
	if training.model != nil {
		training.model.Act(symbol, tokens.String(), training.actions[tokens.String()])
		return
	}

	action, ok := training.actions[tokens.String()]

	if !ok {
		return
	}

	switch action {
	case "enter.bin":
		if !training.desk.Has(symbol, false) {
			if err := training.desk.Enter(symbol, "", false); err != nil {
				training.Error(err)
			}
		}
	case "exit.bin":
		if training.desk.Has(symbol, false) {
			training.desk.Exit(symbol, false)
		}
	default:
		if err := training.desk.Enter(symbol, "", true); err != nil {
			err = training.desk.Exit(symbol, true)

			if err != nil {
				training.Error(err)
			}
		}
	}
}

func (training *Training) reloadPrefixes() {
	training.prefixes = make([][]*Token, 0)
	training.actions = map[string]string{}

	if training.client == nil {
		return
	}

	for key := range training.client.ListKeys("policy/current") {
		rest := strings.Trim(strings.TrimPrefix(key, "policy/current"), "/")
		action := ""

		switch {
		case strings.HasSuffix(rest, "enter.bin"):
			action = "enter.bin"
		case strings.HasSuffix(rest, "exit.bin"):
			action = "exit.bin"
		default:
			continue
		}

		rest = strings.Trim(strings.TrimSuffix(rest, action), "/")
		parts := make([]string, 0)

		for part := range strings.SplitSeq(rest, "/") {
			if part == "" || part == "policy" || part == "current" {
				continue
			}

			parts = append(parts, part)
		}

		if len(parts) == 0 {
			continue
		}

		path := strings.Join(parts, "/")
		training.actions[path] = action

		if training.model != nil {
			training.model.Note(path)
		}

		for i, part := range parts {
			for len(training.prefixes) <= i {
				training.prefixes = append(training.prefixes, nil)
			}

			training.prefixes[i] = append(training.prefixes[i], tokenizer.NewToken(
				[]string{part}, 0.5, 0.5,
			))
		}
	}
}

/*
fork gives one detection its own grid and sentence. Prefixes and actions stay
shared and are only read after reload.
*/
func (training *Training) fork() *Training {
	if training == nil {
		return nil
	}

	return &Training{
		System:     runtime.NewSystem(training.Context(), "training"),
		price:      training.price,
		desk:       training.desk,
		client:     training.client,
		grid:       nmstore.NewGrid(training.Context()),
		prefixes:   training.prefixes,
		actions:    training.actions,
		tokenizers: &sync.Map{},
		policy:     &sync.Map{},
		hub:        training.hub,
	}
}

func admitted(regions []*Token, keys []string) []*Token {
	if len(keys) == 0 || len(regions) == 0 {
		return nil
	}

	left := make(map[string]int, len(keys))

	for _, key := range keys {
		left[key]++
	}

	out := make([]*Token, 0, len(keys))

	for _, region := range regions {
		if left[key(region)] == 0 {
			continue
		}

		left[key(region)]--
		out = append(out, region)
	}

	return out
}
