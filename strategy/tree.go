package strategy

import (
	"context"
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

type Action string

const (
	ActionNone  Action = "none"
	ActionEnter Action = "enter"
	ActionHold  Action = "hold"
	ActionExit  Action = "exit"
)

// seedResistance is the desk baseline for a brand-new leaf.
func seedResistance() float64 {
	return system.Cfg.Learning.ThresholdStart
}

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

			path, action := leafObject(*object.Key, seedPrefix)

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

			decision.Resistance = clampResistance(decision.Resistance)

			if action != "" && decision.Action == "" {
				decision.Action = action
			}

			tree.mu.Lock()
			root := tree.root.Load()
			root, _, _ = root.Insert([]byte(path), decision)
			tree.root.Store(root)
			tree.mu.Unlock()
		}
	}

	return tree
}

/*
Predict returns the agreed action and average resistance under the cut.
disagreed is true when matching leaves name different actions.
*/
func (tree *Tree) Predict(tokens runtime.Tokens) (Action, float64, bool) {
	min := system.Cfg.Learning.MinPathLength

	if tokens.Len() < min {
		return ActionNone, 0, false
	}

	var (
		current    = ActionNone
		disagreed  bool
		resistance float64
		count      int
		prunes     []string
	)

	tree.root.Load().Root().WalkPrefix([]byte(tokens.String()), func(
		key []byte, decision *runtime.Decision,
	) bool {
		if decision == nil {
			return false
		}

		if strings.Count(string(key), "/")+1 < min {
			return false
		}

		action := NewAction(decision.Action)

		if current != ActionNone && current != action {
			disagreed = true
			return false
		}

		current = action

		if decision.Resistance > 0.8 {
			prunes = append(prunes, string(key))
			return false
		}

		resistance += decision.Resistance
		count++
		return false
	})

	for _, prune := range prunes {
		tree.mu.Lock()
		root := tree.root.Load()
		root, _, _ = root.Delete([]byte(prune))
		tree.root.Store(root)
		tree.mu.Unlock()

		if tree.store != nil {
			if err := tree.store.DeletePrefix("model/paths/" + prune + "/"); err != nil {
				errnie.Error(err)
			}
		}
	}

	if disagreed {
		return ActionNone, 0, true
	}

	if count == 0 {
		return ActionNone, 0, false
	}

	resistance /= float64(count)

	if tokens.Last().Brightness <= resistance {
		return ActionNone, 0, false
	}

	return current, resistance, false
}

/*
Update applies a settled ±delta to every decision under path.
*/
func (tree *Tree) Update(path string, delta float64) {
	if path == "" || strings.Count(path, "/")+1 < system.Cfg.Learning.MinPathLength {
		return
	}

	floor := system.Cfg.Learning.ThresholdFloor
	ceil := system.Cfg.Learning.ThresholdCeil

	tree.root.Load().Root().WalkPrefix([]byte(path), func(
		key []byte, decision *runtime.Decision,
	) bool {
		if decision == nil {
			return false
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

		tree.mu.Lock()
		root := tree.root.Load()
		root, _, _ = root.Insert(key, &published)
		tree.root.Store(root)
		tree.mu.Unlock()
		tree.persist(string(key), &published)

		return false
	})
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

	if _, exists := tree.root.Load().Get([]byte(path)); exists {
		return
	}

	tree.Insert(path, &runtime.Decision{
		Action:     action.String(),
		Path:       path,
		Resistance: seedResistance(),
	})
}

// clampResistance keeps a loaded leaf inside the configured band.
func clampResistance(value float64) float64 {
	floor := system.Cfg.Learning.ThresholdFloor
	ceil := system.Cfg.Learning.ThresholdCeil

	if value < floor {
		return floor
	}

	if value > ceil {
		return ceil
	}

	return value
}

func (tree *Tree) Delete(path string) {
	if path == "" {
		return
	}

	tree.mu.Lock()
	root := tree.root.Load()
	root, _, _ = root.Delete([]byte(path))
	tree.root.Store(root)
	tree.mu.Unlock()

	if tree.store != nil {
		if err := tree.store.DeletePrefix("model/paths/" + path + "/"); err != nil {
			errnie.Error(err)
		}
	}
}

func (tree *Tree) persist(path string, decision *runtime.Decision) {
	if tree.store == nil || decision == nil || path == "" {
		return
	}

	action := NewAction(decision.Action)

	if action == ActionNone {
		return
	}

	raw, err := sonic.Marshal(decision)

	if err != nil || len(raw) == 0 {
		errnie.Error(errnie.Err(
			errnie.IO, "[tree] error marshalling decision", err,
		))

		return
	}

	// model/paths/<tokens>/<enter|hold|exit>.bin
	tree.store.WriteAtPrefix(
		"model/paths/"+path+"/"+action.String()+".bin", raw,
	)
}

/*
leafObject maps an S3 key under seedPrefix to the radix path and action.
Canonical: model/paths/R01/M1/enter.bin → ("R01/M1", "enter").
Legacy flat: model/paths/R01/M1.bin → ("R01/M1", "").
*/
func leafObject(objectKey, seedPrefix string) (path, action string) {
	path = strings.TrimPrefix(objectKey, seedPrefix)
	path = strings.TrimSuffix(path, ".json")
	path = strings.TrimSuffix(path, ".bin")
	path = strings.Trim(path, "/")

	if path == "" {
		return "", ""
	}

	slash := strings.LastIndex(path, "/")

	if slash < 0 {
		return path, ""
	}

	tail := path[slash+1:]

	switch tail {
	case "enter", "hold", "exit":
		return path[:slash], tail
	default:
		return path, ""
	}
}
