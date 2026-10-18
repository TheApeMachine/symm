package strategy

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

type Token struct {
	Key        string
	Threshold  float64
	Brightness float64
}

type Tokens []*Token

func (tokens Tokens) String() string {
	out := make([]string, 0)

	for _, token := range tokens {
		out = append(out, token.Key)
	}

	return strings.Join(out, "/")
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
		tokenizers: &sync.Map{},
		hub:        hub,
		policy:     &sync.Map{},
		actions:    map[string]string{},
	}

	training.reloadPrefixes()
	training.Transition(runtime.INIT)
	return training
}

func (training *Training) Step(value *data.Value) *data.Value {
	if value == nil {
		return nil
	}

	grouped := make(map[string][]data.Signal)

	for signal, err := range data.Signals(value) {
		if err != nil {
			training.Error(err)
			continue
		}

		grouped[signal.Symbol] = append(grouped[signal.Symbol], signal)
	}

	for symbol, signals := range grouped {
		found, _ := training.policy.LoadOrStore(symbol, []*Token{})
		tokens := found.([]*Token)
		regions := make([]*Token, 0)

		for _, region := range training.grid.Observe(symbol, signals) {
			regions = append(regions, &Token{
				Key:        fmt.Sprintf("R%02d", region.Key),
				Threshold:  region.Brightness,
				Brightness: region.Brightness,
			})
		}

		seq, _ := training.tokenizers.LoadOrStore(symbol, tokenizer.NewSequitur())
		tok := seq.(*tokenizer.Sequitur)

		admit := training.match(tokens, regions)
		rule, ok := tok.Analyze(admit)

		if ok {
			path := strings.Join(tok.TerminalNames(rule), "/")
			training.act(symbol, Tokens{{Key: path}})
			training.policy.Store(symbol, []*Token{})
			training.tokenizers.Store(symbol, tokenizer.NewSequitur())
			continue
		}

		tokens = append(tokens, regions...)
		training.policy.Store(symbol, tokens)
		training.act(symbol, tokens)

		for _, region := range regions {
			data.AppendVector(
				value,
				[]float64{region.Brightness},
				"train/regions/"+symbol+"/"+region.Key,
			)
		}

		if len(tokens) > 0 {
			keys := make([]float64, len(tokens))
			labels := make([]string, len(tokens))

			for i, t := range tokens {
				keys[i] = t.Brightness
				labels[i] = t.Key
			}

			data.AppendVector(
				value,
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
	keys := make([]string, 0, len(regions))

	for i, region := range regions {
		step := len(sentence) + i

		if len(training.prefixes) == 0 {
			keys = append(keys, region.Key)
			continue
		}

		if step >= len(training.prefixes) {
			continue
		}

		for _, gate := range training.prefixes[step] {
			if region.Key == gate.Key && region.Brightness <= gate.Threshold {
				keys = append(keys, region.Key)
				break
			}
		}
	}

	return keys
}

func (training *Training) act(symbol string, tokens Tokens) {
	action, ok := training.actions[tokens.String()]

	if !ok {
		return
	}

	switch action {
	case "enter.bin":
		training.desk.Enter(symbol)
	case "exit.bin":
		training.desk.Exit(symbol)
	}
}

func (training *Training) reloadPrefixes() {
	training.prefixes = make([][]*Token, 0)
	training.actions = map[string]string{}

	if training.client == nil {
		return
	}

	for _, prefix := range training.client.ListKeys("policy", "current") {
		for _, part := range strings.Split(prefix, "/") {
			training.prefixes = append(training.prefixes, []*Token{
				{Key: part, Threshold: 0.5, Brightness: 0.5},
			})

			if part == "enter.bin" || part == "exit.bin" {
				training.actions[strings.ReplaceAll(strings.ReplaceAll(
					prefix, "policy/current/", "",
				), "/"+part, "")] = part
			}
		}
	}
}
