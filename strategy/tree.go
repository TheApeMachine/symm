package strategy

import (
	"context"
	"strings"
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

	// seedResistance is the desk baseline for a brand-new leaf.
	seedResistance = 0.5
)

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
	store  *store.Client
}

func NewTree(ctx context.Context, seedPrefix string) *Tree {
	ctx, cancel := context.WithCancel(ctx)

	tree := &Tree{
		ctx:    ctx,
		cancel: cancel,
		root:   atomic.Pointer[iradix.Tree[*runtime.Decision]]{},
		store:  store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
	}

	tree.root.Store(iradix.New[*runtime.Decision]())

	for page := range tree.store.List(seedPrefix) {
		for _, object := range page.Contents {
			if object.Key == nil {
				continue
			}

			raw := tree.store.Fetch(ctx, *object.Key)
			decision := &runtime.Decision{}

			if err := sonic.Unmarshal(raw, decision); err != nil {
				errnie.Error(errnie.Err(
					errnie.IO, "[tree] error unmarshalling decision", err,
				))

				continue
			}

			path := strings.TrimSuffix(strings.TrimPrefix(*object.Key, seedPrefix), ".json")
			path = strings.TrimSuffix(path, ".bin")

			if path == "" {
				continue
			}

			root := tree.root.Load()
			root, _, _ = root.Insert([]byte(path), decision)
			tree.root.Store(root)
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

	tree.root.Load().Root().WalkPrefix([]byte(tokens.String()), func(key []byte, decision *runtime.Decision) bool {
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
		root := tree.root.Load()
		root, _, _ = root.Delete([]byte(prune))
		tree.root.Store(root)

		if tree.store != nil {
			tree.store.DeletePrefix("model/paths/" + prune)
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

	tree.root.Load().Root().WalkPrefix([]byte(path), func(key []byte, decision *runtime.Decision) bool {
		if decision == nil {
			return false
		}

		decision.Resistance += delta
		root := tree.root.Load()
		root, _, _ = root.Insert(key, decision)
		tree.root.Store(root)
		tree.persist(string(key), decision)

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

	root := tree.root.Load()
	root, _, _ = root.Insert([]byte(path), decision)
	tree.root.Store(root)
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
		Resistance: seedResistance,
	})
}

func (tree *Tree) Delete(path string) {
	if path == "" {
		return
	}

	root := tree.root.Load()
	root, _, _ = root.Delete([]byte(path))
	tree.root.Store(root)

	if tree.store != nil {
		tree.store.DeletePrefix("model/paths/" + path)
	}
}

func (tree *Tree) persist(path string, decision *runtime.Decision) {
	if tree.store == nil || decision == nil || path == "" {
		return
	}

	raw, err := sonic.Marshal(decision)

	if err != nil || len(raw) == 0 {
		errnie.Error(errnie.Err(
			errnie.IO, "[tree] error marshalling decision", err,
		))

		return
	}

	tree.store.WriteAtPrefix("model/paths/"+path+".bin", raw)
}
