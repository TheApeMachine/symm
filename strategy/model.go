package strategy

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/ui"
)

var _ ui.CognitionSource = (*Model)(nil)

/*
Model is the predictive trie of region-token contexts. It owns the cognition
memory and every primitive that reads or writes it, and hides the adapter
protocol those primitives speak: callers ask for a call, teach an action, or
take a checkpoint in one operation.
*/
type Model struct {
	recall        *cognition.Recall
	trainer       *cognition.Train
	census        *cognition.Census
	snapshot      *cognition.Snapshot
	tree          *cognition.Export
	version       atomic.Uint64
	cachedVersion uint64
	cachedTree    ui.CognitionTreeExport
	hasCache      bool
	treeMu        sync.RWMutex
}

/*
Call is the trie's answer for one context. An empty Winner abstains.
*/
type Call struct {
	Winner     string
	Confidence float64
	Contrast   float64
}

func NewModel() *Model {
	memory := cognition.NewAssociate()

	return &Model{
		recall:   cognition.NewRecall(memory),
		trainer:  cognition.NewTrain(memory),
		census:   cognition.NewCensus(memory),
		snapshot: cognition.NewSnapshot(memory),
		tree:     cognition.NewExport(memory),
	}
}

/*
Recall answers the action the trie takes on context. Stance is the one
action the asker can take — enter while flat, exit while holding — and
keeps the other action from outweighing it on a shared context; under a
stance the action leads only while reinforced above its graded start. An
empty stance lets enter and exit compete (cognition.Recall).
*/
func (model *Model) Recall(context, stance string) (Call, error) {
	reading, err := ask(model.recall, map[string]string{"context": context, "stance": stance}, nil)

	if err != nil {
		return Call{}, errnie.Error(err)
	}

	var call Call

	if call.Winner, _, err = readText(reading, "winner"); err != nil {
		return Call{}, errnie.Error(err)
	}

	if call.Confidence, _, err = readNumber(reading, "confidence"); err != nil {
		return Call{}, errnie.Error(err)
	}

	if call.Contrast, _, err = readNumber(reading, "contrast"); err != nil {
		return Call{}, errnie.Error(err)
	}

	return call, nil
}

/*
Teach grades context with class and signed feedback: positive reinforces,
negative inhibits (cognition.Reinforce). Terminal classes are enter and
exit only — wait is precursor stance (abstention), never a leaf action.
*/
func (model *Model) Teach(context, class string, feedback float64) error {
	if class == actionWait {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[model] wait is not a terminal action; dampen enter or abstain",
			nil,
		))
	}

	if class != actionEnter && class != actionExit {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[model] terminal action must be enter or exit, got \""+class+"\"",
			nil,
		))
	}

	model.version.Add(1)

	_, err := ask(model.trainer, map[string]string{
		"context": context,
		"class":   class,
	}, map[string]float64{
		"feedback": feedback,
		"graded":   core.Unit,
	})

	return errnie.Error(err)
}

/*
Count answers one census figure: "records", "span", or a class name. The
census only publishes classes that introduced a key, so an absent class has
introduced none and counts zero.
*/
func (model *Model) Count(key string) (float64, error) {
	reading, err := ask(model.census, nil, nil)

	if err != nil {
		return 0, errnie.Error(err)
	}

	value, _, err := readNumber(reading, key)
	return value, errnie.Error(err)
}

/*
Checkpoint answers the serialized trie.
*/
func (model *Model) Checkpoint() ([]byte, error) {
	reading, err := ask(model.snapshot, nil, nil)

	if err != nil {
		return nil, errnie.Error(err)
	}

	encoded, ok, err := readText(reading, "model")

	if err != nil {
		return nil, errnie.Error(err)
	}

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[model] cognition snapshot holds no model",
			nil,
		))
	}

	return []byte(encoded), nil
}

/*
CognitionTree exports the trie topology for the UI. A failed export is logged
and shows an empty tree; it never feeds trading.
*/
func (model *Model) CognitionTree() ui.CognitionTreeExport {
	currentVersion := model.version.Load()

	model.treeMu.RLock()
	if model.hasCache && model.cachedVersion == currentVersion {
		cached := model.cachedTree
		model.treeMu.RUnlock()
		return cached
	}
	model.treeMu.RUnlock()

	reading, err := ask(model.tree, nil, nil)

	if err != nil {
		errnie.Error(err)
		return ui.CognitionTreeExport{}
	}

	raw, ok, err := readText(reading, "tree")

	if err != nil {
		errnie.Error(err)
		return ui.CognitionTreeExport{}
	}

	if !ok {
		return ui.CognitionTreeExport{}
	}

	var export ui.CognitionTreeExport

	if err = json.Unmarshal([]byte(raw), &export); err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "[model] cognition tree", err))
		return ui.CognitionTreeExport{}
	}

	model.treeMu.Lock()
	model.cachedVersion = currentVersion
	model.cachedTree = export
	model.hasCache = true
	model.treeMu.Unlock()

	return export
}

/*
ask issues text and numbers to a cognition primitive through one adapter and
answers that adapter for reading.
*/
func ask(
	primitive core.Primitive,
	text map[string]string,
	numbers map[string]float64,
) (*data.Adapter, error) {
	adapter := data.NewAdapter(nil, data.NewState(data.NewMap()))

	if len(text) > 0 {
		issued := data.NewTextMap()

		for key, value := range text {
			issued.Values[key] = value
		}

		for range adapter.Next(data.NewValue(issued)) {
		}

		if err := adapter.Error(); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "[model] cognition text", err))
		}
	}

	if len(numbers) > 0 {
		issued := data.NewOutputMap()

		for key, value := range numbers {
			issued.Values[key] = value
		}

		for range adapter.Next(data.NewValue(issued)) {
		}

		if err := adapter.Error(); err != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "[model] cognition numbers", err))
		}
	}

	for range primitive.Next(data.NewValue(adapter)) {
	}

	if err := primitive.Error(); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "[model] cognition", err))
	}

	if err := adapter.Error(); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "[model] cognition", err))
	}

	return adapter, nil
}

/*
readNumber reads key from an answered adapter. A key the primitive did not
publish is absent (false, nil); the adapter's error state is sticky, so a
published value is taken whenever it arrived, and any other failure is an
error.
*/
func readNumber(adapter *data.Adapter, key string) (float64, bool, error) {
	var values data.Map[float64]

	for pointer := range adapter.Next(data.NewValue(data.NewMap(key, key))) {
		values = *(*data.Map[float64])(pointer)
	}

	value, ok := values.Values[key]

	if err := absence(adapter.Error(), ok); err != nil {
		return 0, false, errnie.Error(errnie.Err(errnie.Validation, "[model] cognition number "+key, err))
	}

	return value, ok, nil
}

func readText(adapter *data.Adapter, key string) (string, bool, error) {
	var values data.Map[string]

	for pointer := range adapter.Next(data.NewValue(data.NewLiteral(key))) {
		values = *(*data.Map[string])(pointer)
	}

	value, ok := values.Values[key]

	if err := absence(adapter.Error(), ok); err != nil {
		return "", false, errnie.Error(errnie.Err(errnie.Validation, "[model] cognition text "+key, err))
	}

	return value, ok, nil
}

/*
absence answers the adapter error that remains a failure: none when the key
arrived, none when the only failure is that the key was never published.
*/
func absence(err error, arrived bool) error {
	if err == nil || arrived {
		return nil
	}

	if errors.Is(err, core.ErrNotHeld) {
		return nil
	}

	return err
}
