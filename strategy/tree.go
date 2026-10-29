package strategy

import (
	"context"
	"math"
	"slices"
	"strings"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
	"golang.org/x/sync/errgroup"
)

type Action string

const (
	ActionNone  Action = "none"
	ActionEnter Action = "enter"
	ActionExit  Action = "exit"
)

func NewAction(action string) Action {
	switch action {
	case "enter":
		return ActionEnter
	case "exit":
		return ActionExit
	default:
		return ActionNone
	}
}

func (action Action) String() string {
	return string(action)
}

/*
Decision carries the predicted action and whether it earned Paper or Virtual.
*/
type Decision struct {
	Action     Action  // ActionEnter, ActionExit, ActionNone
	Virtual    bool    // false = Paper, true = Virtual
	Impulse    float64 // Live force: Brightness * (1 + Coder Confidence)
	Resistance float64 // exp of the path book: the force the live impulse must clear
}

/*
pathBook is one stored path.

Threshold is the path book on the [0,1] scale the cognitive tree renders:
0.50 at the start, profit pulls it toward 0 (solid), loss toward 1 (faint).
It is the recency-weighted mean of per-trade deltas seeded at 0.5, so a
recurring path does not drift with the count of past trades (the popularity
bias the old exp(sum) book had).

Deltas is the sum of the per-trade deltas (−return) and Weights the sum of
the recency weights that fold into the mean, so the book survives restart
without carrying a running product.
*/
type pathValue struct {
	Threshold float64 `json:"threshold"`
	Action    string  `json:"action"`
	Deltas    float64 `json:"res_deltas"`
	Weights   float64 `json:"res_weights"`
}

/*
pathResistance is the force the live impulse must clear: the exponent of the
path book. A profitable path sits near 1 (easy to break), a losing path rises
(hard to break) and, once the book crosses the prune threshold, is retired.
*/
func pathResistance(value *pathValue) float64 {
	if value == nil || value.Threshold < 0 {
		return 1.0
	}
	return math.Exp(value.Threshold)
}

/*
Tree is a radix tree of path keys and their associated data values.
*/
type Tree struct {
	ctx    context.Context
	cancel context.CancelFunc
	root   *iradix.Tree[*pathValue]
	store  *store.Client

	pruneThreshold float64 // retire paths whose book crosses this (a realized loss)
	maxObjects     int     // bound the resident path count
}

/*
NewTree creates a new tree with the given context and seed prefix and restores
it from S3 in a bounded group.
*/
func NewTree(ctx context.Context, seedPrefix string) *Tree {
	ctx, cancel := context.WithCancel(ctx)

	prune := 0.80
	maxObjects := 0
	if cfg := system.Cfg.Learning; cfg != nil {
		if cfg.PathPruneResistance > 0 {
			prune = cfg.PathPruneResistance
		}
		maxObjects = cfg.MaxPathObjects
	}

	tree := &Tree{
		ctx:            ctx,
		cancel:         cancel,
		root:           iradix.New[*pathValue](),
		store:          store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
		pruneThreshold: prune,
		maxObjects:     maxObjects,
	}

	var keys []string
	for page := range tree.store.List(seedPrefix) {
		for _, object := range page.Contents {
			if object.Key == nil {
				continue
			}
			keys = append(keys, *object.Key)
		}
	}

	group, gctx := errgroup.WithContext(ctx)
	group.SetLimit(16)

	type result struct {
		key   []byte
		value *pathValue
	}
	results := make(chan result, len(keys))

	for _, key := range keys {
		key := key
		group.Go(func() error {
			value := tree.store.Download(gctx, key)
			if value == nil || value.Err != nil {
				return nil
			}
			results <- result{
				key:   []byte(strings.TrimPrefix(key, "model/paths/")),
				value: decodePathValue(value),
			}
			return nil
		})
	}

	group.Wait()
	close(results)

	for r := range results {
		if r.value != nil {
			tree.root, _, _ = tree.root.Insert(r.key, r.value)
		}
	}

	return tree
}

/*
Predict evaluates the live pin brightness and predictive coder against the
recency-weighted resistance of the matching paths.
*/
func (tree *Tree) Predict(tokens *tokenizer.Tokens, coder CoderReading) Decision {
	if tokens == nil || tokens.Len() == 0 {
		return Decision{Action: ActionNone}
	}

	prefix := tokens.String()
	action := ActionNone
	// A geometric (recency-weighted) mean of the matching paths' resistances.
	// Summing the log of each path's resistance gives the geometric mean, so a
	// single frequent prefix cannot dominate the sum.
	totalLog := 0.0
	matchCount := 0

	tree.root.Root().WalkPrefix([]byte(prefix), func(key []byte, value *pathValue) bool {
		if value == nil {
			return false
		}

		nodeAction := NewAction(value.Action)

		if action != ActionNone && action != nodeAction {
			action = ActionNone
			return true
		}

		action = nodeAction

		totalLog += math.Log(pathResistance(value))
		matchCount++

		return false
	})

	if matchCount == 0 || action == ActionNone {
		return Decision{
			Action:     ActionEnter,
			Virtual:    true,
			Impulse:    0.0,
			Resistance: 1.0,
		}
	}

	resistance := math.Exp(totalLog / float64(matchCount))

	brightness := tokens.Brightness()
	impulse := 0.0

	if action == ActionEnter {
		if coder.Bullish {
			impulse = brightness * (1.0 + coder.Confidence)
		}
	} else {
		impulse = brightness
	}

	isVirtual := impulse < resistance

	return Decision{
		Action:     action,
		Virtual:    isVirtual,
		Impulse:    impulse,
		Resistance: resistance,
	}
}

/*
UpdateResistance folds one settled round trip into every path that co-signed
the action, then prunes any path whose book crosses the prune threshold and
bounds the resident path count.
*/
func (tree *Tree) UpdateResistance(pathKey string, action Action, returnFraction float64) {
	prefix := []byte(pathKey)
	actionStr := action.String()

	type changed struct {
		key   []byte
		value *pathValue
	}

	var updated []changed

	tree.root.Root().WalkPrefix(prefix, func(key []byte, value *pathValue) bool {
		if value == nil {
			return false
		}

		if NewAction(value.Action) != action {
			return false
		}

		updated = append(updated, changed{key: key, value: foldOutcome(value, actionStr, returnFraction)})
		return false
	})

	if len(updated) == 0 {
		key := []byte(pathKey + "/" + actionStr + ".bin")
		next := &pathValue{
			Threshold: 0.5,
			Action:    actionStr,
			Deltas:    -returnFraction,
			Weights:   1.0,
		}
		tree.root, _, _ = tree.root.Insert(key, next)
		tree.persist(key, next)

		tree.prune()
		return
	}

	for _, c := range updated {
		tree.root, _, _ = tree.root.Insert(c.key, c.value)
		tree.persist(c.key, c.value)
	}

	tree.prune()
}

/*
foldOutcome folds one settled return into a path's recency-weighted book. The
delta is −return (a win pulls the book down, a loss up); the new mean is the
recency-weighted mean of all deltas, which is the path book on the [0,1] scale.
*/
func foldOutcome(value *pathValue, action string, returnFraction float64) *pathValue {
	newDeltas := value.Deltas - returnFraction
	newWeights := value.Weights + 1.0

	return &pathValue{
		Threshold: 0.5 + newDeltas/newWeights,
		Action:    action,
		Deltas:    newDeltas,
		Weights:   newWeights,
	}
}

/*
persist writes a path's serialized value to S3 under the paths prefix. The
threshold is written as an absolute snapshot, matching the cognitive tree's
0.5-seeded "book" contract.
*/
func (tree *Tree) persist(key []byte, value *pathValue) {
	if tree.store == nil {
		return
	}
	tree.store.WriteAtPrefix("model/paths/"+string(key), encodePathValue(value).Bytes())
}

/*
prune retires paths whose book crosses the prune threshold (a realized loss)
from both the tree and S3, and evicts the faintest paths beyond the resident
cap. This is what keeps the prefix walk bounded as the tree grows.
*/
func (tree *Tree) prune() {
	if tree == nil || tree.root == nil {
		return
	}

	if tree.maxObjects <= 0 && tree.pruneThreshold <= 0 {
		return
	}

	type path struct {
		key   []byte
		value *pathValue
	}

	var all []path

	tree.root.Root().Walk(func(key []byte, value *pathValue) bool {
		if value != nil {
			all = append(all, path{key: key, value: value})
		}
		return false
	})

	for _, p := range all {
		if tree.pruneThreshold > 0 && p.value.Threshold > tree.pruneThreshold {
			tree.root, _, _ = tree.root.Delete(p.key)
			if tree.store != nil {
				_ = tree.store.DeletePrefix("model/paths/" + string(p.key))
			}
		}
	}

	if tree.maxObjects > 0 && len(all) > tree.maxObjects {
		// Evict the faintest (lowest-threshold, least informative) paths first.
		slices.SortFunc(all, func(a, b path) int {
			switch {
			case b.value.Threshold > a.value.Threshold:
				return 1
			case b.value.Threshold < a.value.Threshold:
				return -1
			default:
				return 0
			}
		})
		excess := len(all) - tree.maxObjects
		for i := 0; i < excess; i++ {
			tree.root, _, _ = tree.root.Delete(all[i].key)
			if tree.store != nil {
				_ = tree.store.DeletePrefix("model/paths/" + string(all[i].key))
			}
		}
	}
}

/*
Has reports whether any path matching pathKey is stored in the tree.
*/
func (tree *Tree) Has(pathKey string) bool {
	if tree == nil || tree.root == nil || pathKey == "" {
		return false
	}

	found := false

	tree.root.Root().WalkPrefix([]byte(pathKey), func(key []byte, value *pathValue) bool {
		found = true
		return true
	})

	return found
}

/*
Catalog returns all stored keys and their current path books. The book is the
[0,1] threshold the cognitive tree renders (0.5 base, profit→0, loss→1).
*/
func (tree *Tree) Catalog() ([]string, map[string]float64) {
	if tree == nil || tree.root == nil {
		return nil, nil
	}

	keys := make([]string, 0)
	thresholds := make(map[string]float64)

	tree.root.Root().Walk(func(key []byte, value *pathValue) bool {
		k := string(key)
		keys = append(keys, k)

		if value != nil {
			thresholds[k] = value.Threshold
		}

		return false
	})

	slices.Sort(keys)
	return keys, thresholds
}

/*
encodePathValue serializes a path's book, deltas, and weights as a data.Value
layer under the paths prefix. Threshold is the [0,1] book the cognitive tree
consumes; the deltas and weights are the recency-weighted state that rebuilds
it on restart.
*/
func encodePathValue(value *pathValue) *data.Value {
	out := data.NewValue("model", "paths", value.Action)
	out.Set(value.Action, "action")
	out.Set(value.Threshold, "threshold")
	out.Set(value.Deltas, "res_deltas")
	out.Set(value.Weights, "res_weights")
	return out
}

/*
decodePathValue rebuilds a pathValue from a stored object, recovering the
threshold from the recency-weighted deltas when an absolute snapshot is absent.
*/
func decodePathValue(value *data.Value) *pathValue {
	if value == nil || value.Err != nil {
		return nil
	}

	out := &pathValue{
		Action:    value.GetString("action"),
		Deltas:    value.GetFloat64("res_deltas"),
		Weights:   value.GetFloat64("res_weights"),
		Threshold: value.GetFloat64("threshold"),
	}

	if out.Threshold <= 0 && out.Weights > 0 {
		out.Threshold = 0.5 + out.Deltas/out.Weights
	}

	if out.Weights <= 0 {
		out.Threshold = 0.5
	}

	return out
}
