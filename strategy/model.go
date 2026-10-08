package strategy

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/ui"
)

var _ ui.CognitionSource = (*Model)(nil)

/*
Model is the predictive trie of region-token contexts. It owns the cognition
memory and every primitive that reads or writes it: callers ask for a call,
teach an action, or take a checkpoint in one operation.
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
Recall answers the action the trie takes on context.
*/
func (model *Model) Recall(context, stance string) (Call, error) {
	query := &cognition.RecallQuery{
		Context: context,
		Stance:  stance,
	}
	var res *cognition.RecallResult

	for ptr := range model.recall.Next(data.NewValue(unsafe.Pointer(query)).Next(nil)) {
		res = (*cognition.RecallResult)(ptr)
	}

	if err := model.recall.Error(); err != nil {
		return Call{}, errnie.Error(err)
	}

	if res == nil {
		return Call{}, nil
	}

	return Call{
		Winner:     res.Winner,
		Confidence: res.Confidence,
		Contrast:   res.Contrast,
	}, nil
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

	record := &cognition.TrainRecord{
		Context:  context,
		Class:    class,
		Feedback: feedback,
		Graded:   core.Unit,
	}

	for range model.trainer.Next(data.NewValue(unsafe.Pointer(record)).Next(nil)) {
	}

	if err := model.trainer.Error(); err != nil {
		return errnie.Error(err)
	}

	return nil
}

/*
Count answers one census figure: "records", "span", or a class name.
*/
func (model *Model) Count(key string) (float64, error) {
	var census *cognition.CensusResult

	for ptr := range model.census.Next(nil) {
		census = (*cognition.CensusResult)(ptr)
	}

	if err := model.census.Error(); err != nil {
		return 0, errnie.Error(err)
	}

	if census == nil {
		return 0, nil
	}

	if key == "records" {
		return census.Records, nil
	}

	if key == "span" {
		return census.Span, nil
	}

	return census.Classes[key], nil
}

/*
Checkpoint answers the serialized trie.
*/
func (model *Model) Checkpoint() ([]byte, error) {
	var encoded string

	for ptr := range model.snapshot.Next(nil) {
		encoded = *(*string)(ptr)
	}

	if err := model.snapshot.Error(); err != nil {
		return nil, errnie.Error(err)
	}

	if encoded == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[model] cognition snapshot holds no model",
			nil,
		))
	}

	return []byte(encoded), nil
}

/*
CognitionTree exports the trie topology for the UI.
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

	var raw string

	for ptr := range model.tree.Next(nil) {
		raw = *(*string)(ptr)
	}

	if err := model.tree.Error(); err != nil {
		errnie.Error(err)
		return ui.CognitionTreeExport{}
	}

	if raw == "" {
		return ui.CognitionTreeExport{}
	}

	var export ui.CognitionTreeExport

	if err := json.Unmarshal([]byte(raw), &export); err != nil {
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
