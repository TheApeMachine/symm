package strategy

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/data/transport"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
)

const thresholdKey = "model/threshold/current"

const (
	thresholdStart = 0.5
	thresholdStep  = 0.05
	thresholdFloor = 0.05
	thresholdCeil  = 0.95
	virtualPromote = 3
)

/*
Model is the energy gate for a token path. A region matches a path only when
its brightness clears that path's threshold. A profitable exit lowers the
threshold and a loss raises it. Three profitable virtual closes store the
path as enter.bin and exit.bin. Threshold, Wins, Promoted, and Open are the
stored model.
*/
type Model struct {
	*runtime.System `json:"-"`
	*transport.Stream
	mu        sync.Mutex
	desk      *broker.Desk
	client    *store.Client
	Threshold map[string]float64 `json:"threshold"`
	Wins      map[string]int     `json:"wins"`
	Promoted  map[string]bool    `json:"promoted"`
	Open      map[string]string  `json:"open"`
}

func NewModel(
	ctx context.Context, desk *broker.Desk, client *store.Client,
) *Model {
	model := &Model{
		System:    runtime.NewSystem(ctx, "model"),
		desk:      desk,
		client:    client,
		Threshold: map[string]float64{},
		Wins:      map[string]int{},
		Promoted:  map[string]bool{},
		Open:      map[string]string{},
	}

	model.Stream = transport.NewStream(model.Context(), func(value *data.Value) *data.Value {
		body, err := sonic.Marshal(model)

		if err != nil {
			model.Error(err)
			return value
		}

		return data.NewValue("model", "threshold", "current").Append(body, "model")
	})

	value := data.NewValue("model", "threshold", "current")

	for stored := range transport.Next(
		transport.NewFlipFlop(value, model.client),
	) {
		if stored.Err != nil {
			model.Error(stored.Err)
			continue
		}

		stored.Lookup(model, "model")
		break
	}

	return model
}

func (model *Model) Note(path string) {
	if _, ok := model.Threshold[path]; ok {
		return
	}

	model.Threshold[path] = thresholdStart
}

func (model *Model) Ready() bool {
	if model == nil {
		return false
	}

	model.mu.Lock()
	defer model.mu.Unlock()

	return len(model.Threshold) > 0
}

func (model *Model) Match(sentence, regions []*Token) []string {
	model.mu.Lock()
	defer model.mu.Unlock()

	keys := make([]string, 0, len(regions))

	for i, region := range regions {
		if model.clears(len(sentence)+i, region) {
			keys = append(keys, region.Key)
		}
	}

	return keys
}

func (model *Model) Below(sentence, regions []*Token) []*Token {
	model.mu.Lock()
	defer model.mu.Unlock()

	out := make([]*Token, 0)

	for i, region := range regions {
		step := len(sentence) + i

		if model.matches(step, region.Key) && !model.clears(step, region) {
			out = append(out, region)
		}
	}

	return out
}

func (model *Model) matches(step int, key string) bool {
	for path := range model.Threshold {
		tokens := strings.Split(path, "/")

		if step < len(tokens) && tokens[step] == key {
			return true
		}
	}

	return false
}

func (model *Model) clears(step int, region *Token) bool {
	for path := range model.Threshold {
		tokens := strings.Split(path, "/")

		if step >= len(tokens) || tokens[step] != region.Key {
			continue
		}

		if region.Brightness >= model.Threshold[path] {
			return true
		}
	}

	return false
}

/*
Act takes the sentence's stored action. A path with no enter or exit is a
virtual round trip on the simulated wallet. The entry path, not the sentence
at the exit, is what the threshold learns from.
*/
func (model *Model) Act(symbol, path, action string) {
	if model == nil || model.desk == nil || symbol == "" || path == "" {
		return
	}

	switch model.decide(symbol, path, action) {
	case "exit":
		model.settle(symbol, path, false)
	case "enter":
		if err := model.desk.Enter(symbol, false); err != nil {
			model.Error(err)
			return
		}

		io.Copy(model.client, model)
	default:
		if err := model.desk.Enter(symbol, true); err != nil {
			model.settle(symbol, path, true)
			return
		}

		io.Copy(model.client, model)
	}
}

func (model *Model) decide(symbol, path, action string) string {
	model.mu.Lock()
	defer model.mu.Unlock()

	holding := model.Open[holdKey(symbol, false)] != ""

	switch {
	case action == "exit.bin" || (model.Promoted[path] && holding):
		return "exit"
	case action == "enter.bin" || model.Promoted[path]:
		return "enter"
	default:
		return "virtual"
	}
}

func (model *Model) settle(symbol, fallback string, virtual bool) {
	path := model.take(symbol, virtual)

	if path == "" {
		path = fallback
	}

	profit, ok := model.desk.Profit(symbol, virtual)

	if err := model.desk.Exit(symbol, virtual); err != nil {
		model.Error(err)
		return
	}

	if path != "" && ok {
		if model.learn(path, profit, virtual) {
			model.write(path)
		}

		return
	}

	if _, err := io.Copy(model.client, model); err != nil {
		model.Error(err)
	}
}

func (model *Model) learn(path string, profit, virtual bool) bool {
	model.mu.Lock()

	model.Note(path)

	if profit {
		model.Threshold[path] = min(max(
			model.Threshold[path]-thresholdStep, thresholdFloor,
		), thresholdCeil)
	} else {
		model.Threshold[path] = min(max(
			model.Threshold[path]+thresholdStep, thresholdFloor,
		), thresholdCeil)
	}

	promote := false

	if virtual && profit {
		model.Wins[path]++

		if model.Wins[path] >= virtualPromote && !model.Promoted[path] {
			model.Promoted[path] = true
			promote = true
		}
	}

	model.mu.Unlock()

	if _, err := io.Copy(model.client, model); err != nil {
		model.Error(err)
	}

	return promote
}

func (model *Model) write(path string) {
	if model.client == nil {
		return
	}

	for _, action := range []string{"enter.bin", "exit.bin"} {
		value := data.NewValue("policy", "current", path, action)

		if _, err := io.Copy(model.client, value); err != nil {
			model.Error(err)
		}
	}
}

func (model *Model) take(symbol string, virtual bool) string {
	model.mu.Lock()
	defer model.mu.Unlock()

	key := holdKey(symbol, virtual)
	path := model.Open[key]
	delete(model.Open, key)
	return path
}

func holdKey(symbol string, virtual bool) string {
	if virtual {
		return symbol + "\x00virtual"
	}

	return symbol + "\x00paper"
}
