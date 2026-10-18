package strategy

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
)

const thresholdKey = "model/state/current"

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
	mu              sync.Mutex
	desk            *broker.Desk
	client          *store.Client
	Threshold       map[string]float64
	Wins            map[string]int
	Promoted        map[string]bool
	Open            map[string]string
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

	model.load()
	return model
}

func (model *Model) load() {
	if model == nil || model.client == nil {
		return
	}

	buf, err := object(model.Context(), model.client, thresholdKey)

	if errnie.IsNotFound(err) {
		return
	}

	if err != nil {
		model.Error(err)
		return
	}

	stored := data.From(buf)

	for _, gates := range data.Layer[map[string]float64](stored, "threshold") {
		model.Threshold = gates
	}

	for _, wins := range data.Layer[map[string]int](stored, "wins") {
		model.Wins = wins
	}

	for _, promoted := range data.Layer[map[string]bool](stored, "promoted") {
		model.Promoted = promoted
	}

	for _, open := range data.Layer[map[string]string](stored, "open") {
		model.Open = open
	}

	if stored.Err != nil && !errnie.IsNotFound(stored.Err) {
		model.Error(stored.Err)
		return
	}

	if len(model.Threshold) > 0 {
		model.Transition(runtime.READY)
	}
}

func (model *Model) Note(path string) {
	if model == nil || path == "" {
		return
	}

	model.mu.Lock()
	defer model.mu.Unlock()

	model.note(path)
}

func (model *Model) note(path string) {
	if _, ok := model.Threshold[path]; ok {
		return
	}

	model.Threshold[path] = thresholdStart
	model.Transition(runtime.READY)
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

	var virtual bool

	switch model.decide(symbol, path, action) {
	case "exit":
		model.settle(symbol, path, false)
		return
	case "close":
		model.settle(symbol, path, true)
		return
	case "enter":
		if err := model.desk.Enter(symbol, false); err != nil {
			model.Error(err)
			return
		}
	case "virtual":
		if err := model.desk.Enter(symbol, true); err != nil {
			model.Error(err)
			return
		}

		virtual = true
	default:
		return
	}

	model.mu.Lock()
	model.note(path)
	model.Open[holdKey(symbol, virtual)] = path
	recorded := data.NewValue("model", "state", "current")
	recorded.Append(model.Threshold, "threshold")
	recorded.Append(model.Wins, "wins")
	recorded.Append(model.Promoted, "promoted")
	recorded.Append(model.Open, "open")
	model.mu.Unlock()

	model.persist(recorded)
}

func (model *Model) decide(symbol, path, action string) string {
	model.mu.Lock()
	defer model.mu.Unlock()

	paper := model.Open[holdKey(symbol, false)] != "" || model.desk.Holding(symbol, false)
	virtual := model.Open[holdKey(symbol, true)] != "" || model.desk.Holding(symbol, true)

	switch {
	case action == "exit.bin" || (model.Promoted[path] && paper):
		return "exit"
	case action == "enter.bin" || model.Promoted[path]:
		if paper {
			return ""
		}

		return "enter"
	case virtual:
		return "close"
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
		model.learn(path, profit, virtual)
		return
	}

	model.mu.Lock()
	recorded := data.NewValue("model", "state", "current")
	recorded.Append(model.Threshold, "threshold")
	recorded.Append(model.Wins, "wins")
	recorded.Append(model.Promoted, "promoted")
	recorded.Append(model.Open, "open")
	model.mu.Unlock()

	model.persist(recorded)
}

func (model *Model) learn(path string, profit, virtual bool) bool {
	model.mu.Lock()

	model.note(path)

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

	recorded := data.NewValue("model", "state", "current")
	recorded.Append(model.Threshold, "threshold")
	recorded.Append(model.Wins, "wins")
	recorded.Append(model.Promoted, "promoted")
	recorded.Append(model.Open, "open")
	model.mu.Unlock()

	model.persist(recorded)

	if !promote {
		return promote
	}

	for _, action := range []string{"enter.bin", "exit.bin"} {
		value := data.NewValue("policy", "current", path, action)
		model.persist(value)
	}

	return promote
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

func (model *Model) persist(value *data.Value) {
	if model == nil || value == nil {
		return
	}

	if value.Err != nil {
		model.Error(value.Err)
		return
	}

	if model.client == nil {
		return
	}

	if _, err := io.Copy(model.client, value); err != nil {
		model.Error(err)
	}
}

