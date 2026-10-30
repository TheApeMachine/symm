package strategy

import (
	"context"
	"sync"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/store"
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

type Tree struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	root   *iradix.Tree[*data.Value]
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
		root:   iradix.New[*data.Value](),
		store:  store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
	}

	for page := range tree.store.List(seedPrefix) {
		for _, object := range page.Contents {
			value := tree.store.Download(ctx, *object.Key)

			if value == nil || value.Err != nil {
				continue
			}

			tree.root, _, _ = tree.root.Insert([]byte(*object.Key), value)
		}
	}

	return tree
}

/*
Predict the action to take, based on the current prefix.
The prediction is based on all the paths in the tree that share the
current prefix, and whether or not the values that those paths point
at are all agreeing on the same action.
*/
func (tree *Tree) Predict(tokens tokenizer.Tokens) (Action, float64, bool) {
	var (
		current             = ActionNone
		disagreed  bool     = false
		resistance          = 0.0
		count               = 0
		prunes     []string = make([]string, 0)
	)

	tree.root.Root().WalkPrefix([]byte(tokens.String()), func(key []byte, value *data.Value) bool {
		if value == nil || value.Err != nil {
			return false
		}

		if current != ActionNone && current.String() != value.Scope {
			disagreed = true
			return false
		}

		current = NewAction(value.Scope)
		add := value.GetTotal("resistance")

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
Update existing paths with the new resistance.
*/
func (tree *Tree) Update(path string, resistance float64) {
	tree.mu.Lock()
	defer tree.mu.Unlock()

	tree.root.Root().WalkPrefix([]byte(path), func(key []byte, value *data.Value) bool {
		if value == nil || value.Err != nil {
			return false
		}

		value.Append(resistance, "resistance")
		return false
	})
}

/*
Insert writes a path to the tree and S3.
*/
func (tree *Tree) Insert(path string, value *data.Value) {
	tree.mu.Lock()
	defer tree.mu.Unlock()

	tree.root, _, _ = tree.root.Insert([]byte(path), value)
	tree.store.WriteAtPrefix("model/paths/"+path+"/"+value.Scope+".bin", value.Bytes())
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
