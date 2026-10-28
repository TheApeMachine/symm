package strategy

import (
	"context"
	"math"
	"slices"
	"strings"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
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
	Resistance float64 // Path hurdle: compounds with past PnL
}

/*
Tree is a radix tree of path keys and their associated data values.
*/
type Tree struct {
	ctx    context.Context
	cancel context.CancelFunc
	root   *iradix.Tree[*data.Value]
	store  *store.Client
}

/*
NewTree creates a new tree with the given context and seed prefix.
*/
func NewTree(ctx context.Context, seedPrefix string) *Tree {
	ctx, cancel := context.WithCancel(ctx)

	tree := &Tree{
		ctx:    ctx,
		cancel: cancel,
		root:   iradix.New[*data.Value](),
		store:  store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
	}

	// Restore the tree from S3 storage.
	for page := range tree.store.List(seedPrefix) {
		for _, object := range page.Contents {
			if object.Key == nil {
				continue
			}

			value := tree.store.Download(tree.ctx, *object.Key)

			if value == nil || value.Err != nil {
				continue
			}

			tree.root, _, _ = tree.root.Insert(
				[]byte(strings.TrimPrefix(*object.Key, "model/paths/")), value,
			)
		}
	}

	return tree
}

/*
Predict evaluates the live pin brightness and predictive coder against the
accumulated physical resistance of the matching paths in the tree.
*/
func (tree *Tree) Predict(tokens *tokenizer.Tokens, coder CoderReading) Decision {
	if tokens == nil || tokens.Len() == 0 {
		return Decision{Action: ActionNone}
	}

	prefix := tokens.String()
	action := ActionNone
	totalResistance := 0.0
	matchCount := 0

	tree.root.Root().WalkPrefix([]byte(prefix), func(key []byte, value *data.Value) bool {
		if value == nil {
			return false
		}

		// Read the action attached to this stored path
		nodeAction := NewAction(value.GetString("action"))

		// If sub-paths disagree on Enter vs Exit, consensus is broken
		if action != ActionNone && action != nodeAction {
			action = ActionNone
			return true
		}

		action = nodeAction

		resistance := math.Exp(value.GetTotal("resistance"))

		if resistance <= 0 {
			resistance = 1.0
		}

		// Accumulate path resistance
		totalResistance += resistance
		matchCount++

		return false
	})

	if matchCount == 0 || action == ActionNone {
		// No existing path was found, so we start
		// exploring this as a new path.
		return Decision{
			Action:     ActionEnter,
			Virtual:    true,
			Impulse:    0.0,
			Resistance: 1.0,
		}
	}

	// Calculate the average resistance of the matching paths.
	resistance := totalResistance / float64(matchCount)

	// Compute Live Forward Impulse (Pin-toy brightness * Coder forward vector)
	brightness := tokens.Brightness()
	impulse := 0.0

	// For an Enter, the Predictive Coder must be forward-bullish
	if action == ActionEnter {
		if coder.Bullish {
			// Energy * (Unit + Coder Confidence)
			impulse = brightness * (1.0 + coder.Confidence)
		}
	} else {
		// Exit actions track raw pressure directly
		impulse = brightness
	}

	// 3. The Physical Comparison: Impulse vs Resistance
	// If Impulse >= Resistance, the force broke through the hurdle -> Paper (Virtual = false)
	// If Impulse < Resistance, the force is insufficient -> Virtual (Virtual = true)
	isVirtual := impulse < resistance

	return Decision{
		Action:     action,
		Virtual:    isVirtual,
		Impulse:    impulse,
		Resistance: resistance,
	}
}

/*
UpdateResistance rewards or penalizes all paths that participated in the consensus,
or creates the path if it was a first-time exploration.
*/
func (tree *Tree) UpdateResistance(
	pathKey string, action Action, returnFraction float64,
) {
	prefix := []byte(pathKey)
	actionStr := action.String()
	updatedCount := 0

	// Walk all paths under this prefix that co-signed the action
	tree.root.Root().WalkPrefix(prefix, func(key []byte, value *data.Value) bool {
		if value == nil {
			return false
		}

		// Only update paths that actually voted for this action
		if NewAction(value.GetString("action")) != action {
			return false
		}

		// Append the trade outcome delta to this path's event log
		value.Append(actionStr, "action")
		value.Append(-returnFraction, "resistance")

		tree.root, _, _ = tree.root.Insert(key, value)

		if tree.store != nil {
			tree.store.WriteAtPrefix("model/paths/"+string(key), value.Bytes())
		}

		updatedCount++
		return false
	})

	// If no existing paths existed (this was a first-time exploration),
	// seed the new path into the tree.
	if updatedCount == 0 {
		key := []byte(pathKey + "/" + actionStr + ".bin")
		value := data.NewValue("model", "paths", actionStr)
		value.Append(actionStr, "action")
		value.Append(-returnFraction, "resistance")

		tree.root, _, _ = tree.root.Insert(key, value)

		if tree.store != nil {
			tree.store.WriteAtPrefix("model/paths/"+string(key), value.Bytes())
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

	tree.root.Root().WalkPrefix([]byte(pathKey), func(key []byte, value *data.Value) bool {
		found = true
		return true // abort walk early
	})

	return found
}

/*
Catalog returns all stored keys and their current compounded resistances.
*/
func (tree *Tree) Catalog() ([]string, map[string]float64) {
	if tree == nil || tree.root == nil {
		return nil, nil
	}

	keys := make([]string, 0)
	thresholds := make(map[string]float64)

	tree.root.Root().Walk(func(key []byte, value *data.Value) bool {
		k := string(key)
		keys = append(keys, k)

		if value != nil {
			thresholds[k] = math.Exp(value.GetTotal("resistance"))
		}

		return false // continue walk
	})

	slices.Sort(keys)
	return keys, thresholds
}
