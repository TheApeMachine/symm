package strategy

import (
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/ui"
)

var _ ui.CognitionSource = (*Model)(nil)

/*
Model is the predictive trie of region-token contexts. It owns the cognition
memory and every primitive that reads or writes it: callers ask for a call,
teach an action, or take a checkpoint in one operation.
*/
type Model struct {
	engine        *cognition.Engine
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
	return &Model{
		engine: cognition.NewEngine(cognition.Config{}),
	}
}

/*
Recall answers the action the trie takes on context.
*/
func (model *Model) Recall(context, stance string) (Call, error) {
	res, err := model.engine.Evaluate([]byte(context))

	if err != nil {
		return Call{}, errnie.Error(err)
	}

	if stance != "" {
		for _, candidate := range res.Evaluation.Candidates {
			if candidate.Name == stance && candidate.Support > 0 {
				contrast := 0.0

				if res.Evaluation.WinnerClass == stance {
					contrast = res.Evaluation.Contrast
				}

				return Call{
					Winner:     candidate.Name,
					Confidence: candidate.Probability,
					Contrast:   contrast,
				}, nil
			}
		}

		return Call{}, nil
	}

	return Call{
		Winner:     res.Evaluation.WinnerClass,
		Confidence: res.Evaluation.Confidence,
		Contrast:   res.Evaluation.Contrast,
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

	_, err := model.engine.Train([]byte(context), []byte(class), feedback)

	if err != nil {
		return errnie.Error(err)
	}

	return nil
}

/*
Count answers one census figure: "records", "span", or a class name.
*/
func (model *Model) Count(key string) (float64, error) {
	if key == "records" {
		return float64(model.engine.Len()), nil
	}

	if key == "span" {
		return float64(model.engine.Span()), nil
	}

	classes := model.engine.Census()
	return float64(classes[key]), nil
}

/*
Checkpoint answers the serialized trie.
*/
func (model *Model) Checkpoint() ([]byte, error) {
	res, err := model.engine.Snapshot()

	if err != nil {
		return nil, errnie.Error(err)
	}

	if len(res.Model) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[model] cognition snapshot holds no model",
			nil,
		))
	}

	return res.Model, nil
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

	tree := model.engine.TreeExport()
	raw, err := json.Marshal(tree)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "[model] cognition tree marshal", err))
		return ui.CognitionTreeExport{}
	}

	var export ui.CognitionTreeExport

	if err := json.Unmarshal(raw, &export); err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "[model] cognition tree unmarshal", err))
		return ui.CognitionTreeExport{}
	}

	model.treeMu.Lock()
	model.cachedVersion = currentVersion
	model.cachedTree = export
	model.hasCache = true
	model.treeMu.Unlock()

	return export
}
