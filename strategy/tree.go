package strategy

import (
	"context"
	"math"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/system"
)

/*
Action names the leaf kind stored on path decisions — separate from runtime
Decision.Action strings so Predict can disagree-check enter/hold/exit leaves.
*/
type Action string

const (
	ActionNone  Action = "none"
	ActionEnter Action = "enter"
	ActionHold  Action = "hold"
	ActionExit  Action = "exit"
)

/*
seedResistance seeds unexplored paths at ThresholdStart so cold explore still
faces a brightness bar without inheriting zero and firing on first flicker.
*/
func seedResistance() float64 {
	return system.Cfg.Learning.ThresholdStart
}

/*
NewAction normalizes persisted decision.Action and S3 filenames into typed leaves.
*/
func NewAction(action string) Action {
	switch action {
	case "enter":
		return ActionEnter
	case "hold":
		return ActionHold
	case "exit":
		return ActionExit
	default:
		return ActionNone
	}
}

/*
String writes the leaf filename segment (enter/hold/exit) for tree.persist keys.
*/
func (action Action) String() string {
	return string(action)
}

/*
Tree is a path → Decision radix. Leaves are decisions, not a separate book type.
*/
type Tree struct {
	ctx    context.Context
	cancel context.CancelFunc
	root   atomic.Pointer[iradix.Tree[*runtime.Decision]]
	mu     sync.Mutex
	store  *store.Client
}

const pathLeafPrefix = "model/paths/"

/*
leafObjectKey is the S3 object for one radix path — same segments as the trie key.
*/
func leafObjectKey(path string) string {
	return pathLeafPrefix + path + ".bin"
}

/*
NewTree hydrates the radix from model/paths/ on boot — first writer wins per
path so duplicate objects for the same trie key do not clobber leaves.
*/
func NewTree(ctx context.Context, seedPrefix string, client *store.Client) *Tree {
	ctx, cancel := context.WithCancel(ctx)

	tree := &Tree{
		ctx:    ctx,
		cancel: cancel,
		root:   atomic.Pointer[iradix.Tree[*runtime.Decision]]{},
		store:  client,
	}

	tree.root.Store(iradix.New[*runtime.Decision]())

	for page := range tree.store.List(seedPrefix) {
		for _, object := range page.Contents {
			if object.Key == nil {
				continue
			}

			path := strings.TrimPrefix(*object.Key, seedPrefix)
			path = strings.TrimSuffix(path, ".bin")

			if path == "" {
				continue
			}

			raw, err := tree.store.Download(ctx, *object.Key)

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.IO, "[tree] error downloading decision", err,
				))

				continue
			}

			decision := &runtime.Decision{}

			if err := sonic.Unmarshal(raw, decision); err != nil {
				errnie.Error(errnie.Err(
					errnie.IO, "[tree] error unmarshalling decision", err,
				))

				continue
			}

			floor := system.Cfg.Learning.ThresholdFloor
			ceil := system.Cfg.Learning.ThresholdCeil
			decision.Resistance = math.Min(
				ceil,
				math.Max(floor, decision.Resistance),
			)

			tree.mu.Lock()
			root := tree.root.Load()

			// First writer wins — matches Ensure. Duplicate action files
			// from a concurrent Ensure race must not clobber the leaf.
			if _, exists := root.Get([]byte(path)); !exists {
				root, _, _ = root.Insert([]byte(path), decision)
				tree.root.Store(root)
			}

			tree.mu.Unlock()
		}
	}

	return tree
}

/*
Predict reads the exact path leaf. WalkPrefix would also visit exit extensions
of an enter key (and byte-prefixes such as R01 vs R010) and treat them as the
same decision. Tail brightness must beat that leaf; resistance above 0.8 prunes it.

The third return is matched: true when a leaf existed for this path. Matched but
ActionNone means resistance vetoed or pruned the leaf — Model must not treat that
as an unexplored cold path and enter anyway.
*/
func (tree *Tree) Predict(tokens runtime.Tokens) (Action, float64, bool) {
	min := system.Cfg.Learning.MinPathLength

	if tokens.Len() < min {
		return ActionNone, 0, false
	}

	path := tokens.String()
	decision, ok := tree.root.Load().Get([]byte(path))

	if !ok || decision == nil {
		return ActionNone, 0, false
	}

	if decision.Resistance > 0.8 {
		// Tombstone at ceil — Delete would make matched=false next tick so
		// Model treats a proven loser as a cold path and Ensure reseeds it.
		ceil := system.Cfg.Learning.ThresholdCeil
		if decision.Resistance < ceil {
			capped := *decision
			capped.Resistance = ceil
			tree.mu.Lock()
			root := tree.root.Load()
			root, _, _ = root.Insert([]byte(path), &capped)
			tree.root.Store(root)
			tree.mu.Unlock()
			tree.persist(path, &capped)
			return ActionNone, capped.Resistance, true
		}

		return ActionNone, decision.Resistance, true
	}

	if tokens.Last().Brightness <= decision.Resistance {
		return ActionNone, decision.Resistance, true
	}

	return NewAction(decision.Action), decision.Resistance, true
}

/*
Update nudges resistance on the exact path leaf after a settled position proves
or disproves that grammar. Extensions of the path are other decisions.
*/
func (tree *Tree) Update(path string, delta float64) {
	if path == "" || strings.Count(path, "/")+1 < system.Cfg.Learning.MinPathLength {
		return
	}

	floor := system.Cfg.Learning.ThresholdFloor
	ceil := system.Cfg.Learning.ThresholdCeil

	tree.mu.Lock()
	root := tree.root.Load()
	decision, ok := root.Get([]byte(path))

	if !ok || decision == nil {
		tree.mu.Unlock()
		return
	}

	next := *decision
	next.Resistance += delta

	if next.Resistance < floor {
		next.Resistance = floor
	}

	if next.Resistance > ceil {
		next.Resistance = ceil
	}

	published := next
	root, _, _ = root.Insert([]byte(path), &published)
	tree.root.Store(root)
	tree.mu.Unlock()
	tree.persist(path, &published)
}

/*
Insert writes a decision leaf under path.
*/
func (tree *Tree) Insert(path string, decision *runtime.Decision) {
	if path == "" || decision == nil {
		return
	}

	if strings.Count(path, "/")+1 < system.Cfg.Learning.MinPathLength {
		return
	}

	tree.mu.Lock()
	root := tree.root.Load()
	root, _, _ = root.Insert([]byte(path), decision)
	tree.root.Store(root)
	tree.mu.Unlock()
	tree.persist(path, decision)
}

/*
Ensure seeds a leaf once. Re-settling must Update, never wipe resistance.
*/
func (tree *Tree) Ensure(path string, action Action) {
	if path == "" || action == ActionNone {
		return
	}

	if strings.Count(path, "/")+1 < system.Cfg.Learning.MinPathLength {
		return
	}

	decision := &runtime.Decision{
		Action:     action.String(),
		Path:       path,
		Resistance: seedResistance(),
	}

	tree.mu.Lock()
	root := tree.root.Load()

	if _, exists := root.Get([]byte(path)); exists {
		tree.mu.Unlock()
		return
	}

	root, _, _ = root.Insert([]byte(path), decision)
	tree.root.Store(root)
	tree.mu.Unlock()
	tree.persist(path, decision)
}

/*
Delete drops one path leaf from memory and its action object. DeletePrefix would
also remove descendant paths that share the key as a byte prefix.
*/
func (tree *Tree) Delete(path string) {
	if path == "" {
		return
	}

	tree.mu.Lock()
	root := tree.root.Load()
	decision, _ := root.Get([]byte(path))
	root, _, _ = root.Delete([]byte(path))
	tree.root.Store(root)
	tree.mu.Unlock()

	if tree.store == nil || decision == nil {
		return
	}

	action := NewAction(decision.Action)

	if action == ActionNone {
		return
	}

	if err := tree.store.Delete(leafObjectKey(path)); err != nil {
		errnie.Error(err)
	}
}

/*
persist snapshots each leaf to model/paths/{path}.bin — trie key and prefix match.
*/
func (tree *Tree) persist(path string, decision *runtime.Decision) {
	if tree.store == nil || decision == nil || path == "" {
		return
	}

	if NewAction(decision.Action) == ActionNone {
		return
	}

	raw, err := sonic.Marshal(decision)

	if err != nil || len(raw) == 0 {
		errnie.Error(errnie.Err(
			errnie.IO, "[tree] error marshalling decision", err,
		))

		return
	}

	tree.store.WriteAtPrefix(leafObjectKey(path), raw)
}
