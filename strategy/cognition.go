package strategy

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

/*
CognitionTree is the prefix matcher over the paths the Trainer published:
each stored key is <token>/<token>/.../<action>.bin, so a path is a prefix
of its action key. The Trainer keys a policy by the path symbolTape.advance
reported when the excursion's entry (or exit) happened, and every such path
is a suffix of the symbol's token stream at that frame: a completed rule's
expansion ends on the token just appended, and the fallback is the last two
tokens. Live, the tree is therefore asked for the deepest stored path that
the recent tokens end on, never shorter than minDepth tokens.
*/
type CognitionTree struct {
	policy   *policy
	minDepth int
	maxDepth int
}

/*
CognitionMatch is the stored path the recent tokens ended on, its depth in
tokens, and the action its policies agree on ("" where they disagree).
*/
type CognitionMatch struct {
	Path   string `json:"path"`
	Depth  int    `json:"depth"`
	Action string `json:"action"`
}

/*
LoadCognitionTree reads the newest published policy epoch through reader.
No published epoch is an empty tree (paper trading stays off); an
unreachable store, a malformed policy, or a policy epoch trained under other
gate settings than run (data.PolicyGate) is an error: the run's tapes would
propose other chords than the ones its paths were learned from.
*/
func LoadCognitionTree(reader *store.Reader, minDepth int, run *system.Resonance) (*CognitionTree, error) {
	if reader == nil {
		return nil, errnie.Err(errnie.Validation, "[cognition] no store reader to load policies from", nil)
	}

	if err := run.Validate(); err != nil {
		return nil, errnie.Err(errnie.Validation, "[cognition] resonance configuration", err)
	}

	loaded, err := loadPolicy(reader)

	if err != nil {
		return nil, err
	}

	if want := gateSettings(run); loaded.epoch != 0 && !loaded.gate.Same(want) {
		return nil, errnie.Err(errnie.Validation, fmt.Sprintf(
			"[cognition] newest policy epoch %d was trained under %s; this run is configured for %s: retrain or change resonance.*",
			loaded.epoch, loaded.gate, want,
		), nil)
	}

	return newCognitionTree(loaded, minDepth)
}

func newCognitionTree(loaded *policy, minDepth int) (*CognitionTree, error) {
	if minDepth < 1 {
		return nil, errnie.Err(errnie.Validation, fmt.Sprintf(
			"[cognition] learning.minimum_path_confidence %d must be at least 1", minDepth,
		), nil)
	}

	tree := &CognitionTree{policy: loaded, minDepth: minDepth}

	for path := range loaded.actions {
		tree.maxDepth = max(tree.maxDepth, len(strings.Split(path, "/")))
	}

	return tree, nil
}

/*
Empty reports whether no path is stored: there is no model to trade.
*/
func (tree *CognitionTree) Empty() bool { return len(tree.policy.actions) == 0 }

func (tree *CognitionTree) Epoch() uint64 { return tree.policy.epoch }

func (tree *CognitionTree) Paths() int { return len(tree.policy.actions) }

/*
Gate is the gate the tree's policy epoch was trained under (zero when empty).
*/
func (tree *CognitionTree) Gate() data.PolicyGate { return tree.policy.gate }

/*
Depth is the longest stored path in tokens: how much of a token stream a
match can ever need.
*/
func (tree *CognitionTree) Depth() int { return tree.maxDepth }

/*
Match returns the deepest stored path that tokens end on.
*/
func (tree *CognitionTree) Match(tokens []string) (CognitionMatch, bool) {
	for depth := min(len(tokens), tree.maxDepth); depth >= tree.minDepth; depth-- {
		path := strings.Join(tokens[len(tokens)-depth:], "/")

		if action, ok := tree.policy.actions[path]; ok {
			return CognitionMatch{Path: path, Depth: depth, Action: action}, true
		}
	}

	return CognitionMatch{}, false
}

/*
Metrics are the stored data.PolicyKeys values of path's policy for action.
*/
func (tree *CognitionTree) Metrics(path, action string) (map[string]float64, bool) {
	metrics, ok := tree.policy.metrics[path+"/"+action+".bin"]
	return metrics, ok
}

/*
Export lists every stored path with the values its policy stored (visits,
mean net gain per visit, confidence 0..1); a key without them is an error.
*/
func (tree *CognitionTree) Export() (ui.CognitionTreeExport, error) {
	branches := make([]ui.TrieBranchJSON, 0, len(tree.policy.keys))

	for _, key := range tree.policy.keys {
		parts := strings.Split(strings.TrimSuffix(key, ".bin"), "/")

		if len(parts) < 2 {
			return ui.CognitionTreeExport{}, errnie.Err(errnie.UnprocessableContent, "[cognition] policy key "+key+" has no path", nil)
		}

		metrics, ok := tree.policy.metrics[key]

		if !ok {
			return ui.CognitionTreeExport{}, errnie.Err(errnie.Internal, "[cognition] policy key "+key+" has no stored metrics", nil)
		}

		branches = append(branches, ui.TrieBranchJSON{
			Hash:       strings.Join(parts[:len(parts)-1], "/"),
			Depth:      len(parts) - 1,
			Visits:     int(metrics["visits"]),
			MeanEdge:   metrics["gain"],
			Confidence: metrics["confidence"],
			Policy:     strings.ToUpper(parts[len(parts)-1]),
		})
	}

	return ui.CognitionTreeExport{Keys: tree.policy.keys, Branches: branches}, nil
}

/*
CognitionFrame is the CognitionTree make run loaded, as the dashboard's
cognitive tree tab draws it: the policy epoch (a JSON string: it exceeds a
JavaScript number) and its path count, whether paper trading acts on it,
the gate it was trained under (null when no epoch is loaded), every stored
key (<token>/.../<action>.bin) and per key its stored visits, mean gain and
confidence (Export). An empty tree is a frame too: Paths 0, Trading false.
*/
type CognitionFrame struct {
	Epoch    uint64              `json:"epoch,string"`
	Paths    int                 `json:"paths"`
	Trading  bool                `json:"trading"`
	Gate     *CognitionGate      `json:"gate"`
	Keys     []string            `json:"keys"`
	Branches []ui.TrieBranchJSON `json:"branches"`
}

/*
CognitionGate is a data.PolicyGate on the wire.
*/
type CognitionGate struct {
	TickEpoch    uint64  `json:"tickEpoch,string"`
	Enabled      bool    `json:"enabled"`
	SurpriseZ    float64 `json:"surpriseZ"`
	Warmup       string  `json:"warmup"`
	LearningRate float64 `json:"learningRate"`
}

/*
CognitionMatchFrame is one symbol's recent tokens meeting the tree after an
appended chord: the deepest stored path they end on (Match, its Depth in
tokens and Action, "" where stored policies disagree) when Stored.
*/
type CognitionMatchFrame struct {
	Symbol string   `json:"symbol"`
	Tick   uint64   `json:"tick"`
	Tokens []string `json:"tokens"`
	Stored bool     `json:"stored"`
	Match  string   `json:"match"`
	Depth  int      `json:"depth"`
	Action string   `json:"action"`
}

/*
PublishTree io.Copy's tree's CognitionFrame to w (the hub's Retained writer).
*/
func PublishTree(w io.Writer, tree *CognitionTree, trading bool) error {
	if w == nil || tree == nil {
		return errnie.Err(errnie.Validation, "[cognition] publishing the tree needs a writer and a tree", nil)
	}

	export, err := tree.Export()

	if err != nil {
		return err
	}

	frame := CognitionFrame{
		Epoch: tree.Epoch(), Paths: tree.Paths(), Trading: trading,
		Keys: export.Keys, Branches: export.Branches,
	}

	if frame.Keys == nil {
		frame.Keys = []string{}
	}

	if !tree.Empty() {
		gate := tree.Gate()
		frame.Gate = &CognitionGate{
			TickEpoch: gate.TickEpoch, Enabled: gate.Enabled, SurpriseZ: gate.SurpriseZ,
			Warmup: gate.Warmup.String(), LearningRate: gate.LearningRate,
		}
	}

	return writeCognition(w, data.CognitionTreeRole, data.PolicyScope, frame, 0, 0)
}

/*
writeCognition io.Copy's one cognition Value carrying payload as JSON.
*/
func writeCognition(w io.Writer, role, scope string, payload any, tick, timestamp uint64) error {
	buf, err := json.Marshal(payload)

	if err != nil {
		return errnie.Err(errnie.Internal, "[cognition] "+role+" frame of "+scope, err)
	}

	out := data.NewValue(data.CognitionOrigin, role, scope)
	out.Tick, out.Timestamp = tick, timestamp
	out.FormatPrefix()
	out.AppendJSON(buf, data.LayerCognition)

	if _, err := io.Copy(w, out); err != nil {
		return errnie.Err(errnie.IO, "[cognition] "+role+" frame of "+scope+" to the hub", err)
	}

	return nil
}
