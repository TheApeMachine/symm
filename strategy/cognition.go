package strategy

import (
	"context"
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
	problem  *CognitionProblem
	// seeded is the chord gate continuing the policy epoch's coder state
	// (nil when the gate is off or the tree has a Problem): the live tapes
	// gate as train would have gated the same ticks next.
	seeded *chordGate
}

/*
CognitionProblem is why make run holds no model to trade: Level
ProblemError (a stored model it cannot use: malformed, or trained under
other gate settings) or ProblemWarning (none stored), the exact Reason, and
the Fix. The run goes on as a view (Hindsight, Workbench, the dashboards);
only paper trading stays off. The dashboard shows it as a banner.
*/
type CognitionProblem struct {
	Level  string `json:"level"`
	Reason string `json:"reason"`
	Fix    string `json:"fix"`
}

const (
	ProblemError   = "error"
	ProblemWarning = "warning"
)

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
Only an unreachable store (an IO failure: everything make run serves reads
it) or invalid run settings are errors. A model make run cannot trade is an
empty tree with its Problem: none published (a warning), or a malformed
policy, or one trained under other gate settings than run (data.PolicyGate),
whose paths were learned from other chords than the run's tapes would
propose, or a gated epoch whose coder state is missing or does not fit
(errors): its gate would start cold, not where train left it.
*/
func LoadCognitionTree(ctx context.Context, reader *store.Reader, minDepth int, run *system.Resonance) (*CognitionTree, error) {
	if reader == nil {
		return nil, errnie.Err(errnie.Validation, "[cognition] no store reader to load policies from", nil)
	}

	if err := run.Validate(); err != nil {
		return nil, errnie.Err(errnie.Validation, "[cognition] resonance configuration", err)
	}

	if minDepth < 1 {
		return nil, errnie.Err(errnie.Validation, fmt.Sprintf(
			"[cognition] learning.minimum_path_confidence %d must be at least 1", minDepth,
		), nil)
	}

	loaded, err := loadPolicy(reader)

	switch {
	case err != nil && errnie.IsIO(err):
		return nil, err
	case err != nil:
		return unusableTree(minDepth, &CognitionProblem{
			Level:  ProblemError,
			Reason: "the newest stored policy is malformed: " + err.Error(),
			Fix:    "re-run detect and train on the tick epoch",
		})
	case loaded.epoch == 0:
		return unusableTree(minDepth, &CognitionProblem{
			Level:  ProblemWarning,
			Reason: "no trained policy is stored under policy/",
			Fix:    "run detect and train on a collected tick epoch",
		})
	}

	if want := gateSettings(run); !loaded.gate.Same(want) {
		return unusableTree(minDepth, &CognitionProblem{
			Level: ProblemError,
			Reason: fmt.Sprintf(
				"newest policy epoch %d was trained under %s; this run is configured for %s",
				loaded.epoch, loaded.gate, want,
			),
			Fix: fmt.Sprintf(
				"re-run detect and train on the tick epoch %d (or set resonance.* back to what it was trained under)",
				loaded.gate.TickEpoch,
			),
		})
	}

	tree, err := newCognitionTree(loaded, minDepth)

	if err != nil || !loaded.gate.Enabled {
		return tree, err
	}

	return seedTree(ctx, reader, tree, run, minDepth)
}

/*
seedTree loads tree's policy epoch's coder state into the gate run trades
with. An unreachable store is an error; a missing or unusable state is the
tree's Problem.
*/
func seedTree(ctx context.Context, reader *store.Reader, tree *CognitionTree, run *system.Resonance, minDepth int) (*CognitionTree, error) {
	gate := tree.Gate()
	fix := fmt.Sprintf("re-run detect and train on the tick epoch %d", gate.TickEpoch)

	states, err := policyCoderStates(stateSource{keys: reader.Keys, download: reader.Download}, gate)

	switch {
	case err != nil && errnie.IsIO(err):
		return nil, err
	case err != nil:
		return unusableTree(minDepth, &CognitionProblem{
			Level: ProblemError, Reason: fmt.Sprintf("policy epoch %d's coder state is unusable: %v", gate.Epoch, err), Fix: fix,
		})
	case states == nil:
		return unusableTree(minDepth, &CognitionProblem{
			Level: ProblemError,
			Reason: fmt.Sprintf(
				"policy epoch %d stores no coder state (trained before coder state was saved): its gate would start cold, not where train left it",
				gate.Epoch,
			),
			Fix: fix,
		})
	}

	seeded, err := newChordGate(ctx, run)

	if err != nil {
		return nil, err
	}

	if err := seeded.seed(states); err != nil {
		return unusableTree(minDepth, &CognitionProblem{
			Level: ProblemError, Reason: fmt.Sprintf("policy epoch %d's coder state does not fit this run: %v", gate.Epoch, err), Fix: fix,
		})
	}

	errnie.Info(fmt.Sprintf("[cognition] continuing the coder state of %d symbols from policy epoch %d", len(states), gate.Epoch))
	tree.seeded = seeded
	return tree, nil
}

/*
unusableTree is the empty tree make run holds when it has no model to trade,
carrying why.
*/
func unusableTree(minDepth int, problem *CognitionProblem) (*CognitionTree, error) {
	tree, err := newCognitionTree(&policy{
		keys:    make([]string, 0),
		actions: make(map[string]string),
		metrics: make(map[string]map[string]float64),
	}, minDepth)

	if err != nil {
		return nil, err
	}

	tree.problem = problem
	return tree, nil
}

/*
Problem is why the tree holds no model to trade, nil when it holds one.
*/
func (tree *CognitionTree) Problem() *CognitionProblem { return tree.problem }

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
confidence (Export), and the Problem (null when the tree holds a model): why
make run has nothing to trade. An empty tree is a frame too: Paths 0,
Trading false.
*/
type CognitionFrame struct {
	Epoch    uint64              `json:"epoch,string"`
	Paths    int                 `json:"paths"`
	Trading  bool                `json:"trading"`
	Gate     *CognitionGate      `json:"gate"`
	Keys     []string            `json:"keys"`
	Branches []ui.TrieBranchJSON `json:"branches"`
	Problem  *CognitionProblem   `json:"problem"`
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
		Keys: export.Keys, Branches: export.Branches, Problem: tree.Problem(),
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
