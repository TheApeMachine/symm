package strategy

import (
	"context"
	"strings"
	"sync"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
)

type Action string

const (
	ActionNone  Action = "none"
	ActionEnter Action = "enter"
	ActionHold  Action = "hold"
	ActionExit  Action = "exit"
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
Regime scopes which action leaves may vote under a cut. Flat books only see
enter; open lots only see hold/exit — so exploration holds cannot disagree
with entry leaves, and old exits cannot silence a flat-book enter.
*/
type Regime struct {
	open bool
}

func FlatRegime() Regime { return Regime{} }

func OpenRegime() Regime { return Regime{open: true} }

func (regime Regime) Allows(action Action) bool {
	if regime.open {
		return action == ActionHold || action == ActionExit
	}

	return action == ActionEnter
}

type Tree struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	root   *iradix.Tree[*PathBook]
	store  *store.Client
}

/*
NewTree creates a new tree with the given context and seed prefix and restores
it from S3 in a bounded group.
*/
func NewTree(ctx context.Context, seedPrefix string) *Tree {
	ctx, cancel := context.WithCancel(ctx)

	tree := &Tree{
		ctx:    ctx,
		cancel: cancel,
		root:   iradix.New[*PathBook](),
		store:  store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
	}

	for page := range tree.store.List(seedPrefix) {
		for _, object := range page.Contents {
			if object.Key == nil {
				continue
			}

			raw := tree.store.Fetch(ctx, *object.Key)
			book := DecodePathBook(raw)

			if book == nil {
				continue
			}

			path := PathFromStorageKey(*object.Key)

			if path == "" {
				continue
			}

			tree.root, _, _ = tree.root.Insert([]byte(path), book)
		}
	}

	return tree
}

/*
Predict the action under the current cut, only among leaves allowed by
regime. Same-regime leaves must agree; out-of-regime leaves are ignored.
*/
func (tree *Tree) Predict(
	tokens tokenizer.Tokens, regime Regime,
) (Action, float64, bool) {
	min := system.Cfg.Learning.MinPathLength

	if tokens.Len() < min {
		return ActionNone, 0.0, false
	}

	var (
		current    = ActionNone
		disagreed  bool
		resistance = 0.0
		count      = 0
		prunes     []string
	)

	tree.root.Root().WalkPrefix([]byte(tokens.String()), func(key []byte, book *PathBook) bool {
		if book == nil {
			return false
		}

		if strings.Count(string(key), "/")+1 < min {
			return false
		}

		action := NewAction(book.Action)

		if !regime.Allows(action) {
			return false
		}

		if current != ActionNone && current != action {
			disagreed = true
			return false
		}

		current = action
		add := book.Resistance

		if add > 0.8 {
			prunes = append(prunes, string(key))
		} else {
			resistance += add
			count++
		}

		return false
	})

	for _, prune := range prunes {
		tree.root, _, _ = tree.root.Delete([]byte(prune))

		if tree.store != nil {
			tree.store.DeletePrefix("model/paths/" + prune)
		}
	}

	if disagreed {
		return ActionNone, 0.0, true
	}

	if count == 0 {
		return ActionNone, 0.0, false
	}

	resistance /= float64(count)

	if tokens.Last().Brightness <= resistance {
		return ActionNone, 0.0, false
	}

	return current, resistance, false
}

/*
Update existing paths with the new resistance delta.
*/
func (tree *Tree) Update(path string, resistance float64) {
	if path == "" {
		return
	}

	if strings.Count(path, "/")+1 < system.Cfg.Learning.MinPathLength {
		return
	}

	tree.mu.Lock()
	defer tree.mu.Unlock()

	tree.root.Root().WalkPrefix([]byte(path), func(key []byte, book *PathBook) bool {
		if book == nil {
			return false
		}

		book.Apply(resistance)

		if tree.store != nil {
			if raw := book.Bytes(); len(raw) > 0 {
				tree.store.WriteAtPrefix(book.StorageKey(string(key)), raw)
			}
		}

		return false
	})
}

/*
Insert writes a path to the tree and S3.
*/
func (tree *Tree) Insert(path string, book *PathBook) {
	if path == "" || book == nil {
		return
	}

	if strings.Count(path, "/")+1 < system.Cfg.Learning.MinPathLength {
		return
	}

	tree.mu.Lock()
	defer tree.mu.Unlock()

	tree.root, _, _ = tree.root.Insert([]byte(path), book)

	if tree.store != nil {
		if raw := book.Bytes(); len(raw) > 0 {
			tree.store.WriteAtPrefix(book.StorageKey(path), raw)
		}
	}
}

/*
Ensure inserts a seeded leaf only when the path is missing. Re-settling the
same virtual episode must Update resistance, never wipe the book back to a
blank value (which would restart near 0 and clear the paper floor).
*/
func (tree *Tree) Ensure(path string, action Action) {
	if path == "" || action == ActionNone {
		return
	}

	if strings.Count(path, "/")+1 < system.Cfg.Learning.MinPathLength {
		return
	}

	tree.mu.Lock()
	_, exists := tree.root.Get([]byte(path))
	tree.mu.Unlock()

	if exists {
		return
	}

	tree.Insert(path, NewPathBook(action))
}

/*
Delete deletes a path from the tree and S3.
*/
func (tree *Tree) Delete(path string) {
	tree.mu.Lock()
	defer tree.mu.Unlock()

	tree.root, _, _ = tree.root.Delete([]byte(path))
	tree.store.DeletePrefix("model/paths/" + path)
}
