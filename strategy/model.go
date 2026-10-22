package strategy

import (
	"context"
	"io"
	"sync"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

const (
	// maxPathLength bounds the token path so trie keys stay at a
	// granularity that can recur in the market.
	maxPathLength = 8

	// minPathLength is the shortest path the model will act on.
	// Shorter paths are too specific to a single tick to be reliable.
	minPathLength = 3

	// promotionWins is the number of profitable closes required to
	// promote an explored path into the current policy tree.
	promotionWins = 3

	// maxLosses is the number of losing closes that retires an
	// explored path so it is no longer re-entered.
	maxLosses = 3
)

// Desk is the trading surface the Model acts through.
// *broker.Desk satisfies this interface.
type Desk interface {
	Has(symbol string, virtual bool) bool
	Enter(symbol string, path string, virtual bool) error
	Exit(symbol string, path string, virtual bool) error
	Virtual(symbol string) *broker.Position
	Positions(open bool) []*broker.Position
	Profit(symbol string, virtual bool) (bool, bool)
	ClosedVirtuals() []*broker.Position
	Drop(symbol string)
}

type Model struct {
	*runtime.System `json:"-"`
	mu             sync.Mutex
	desk           Desk
	store          *store.Client
	hub            io.Writer
	tree           *iradix.Tree[*data.Value]
	explored       *iradix.Tree[*data.Value]
	sequencers     *sync.Map
	paths          *sync.Map
	settled        *sync.Map
}

func NewModel(
	ctx context.Context, desk Desk, client *store.Client,
	hub io.Writer,
) *Model {
	model := &Model{
		System:     runtime.NewSystem(ctx, "model"),
		desk:       desk,
		store:      client,
		hub:        hub,
		tree:       iradix.New[*data.Value](),
		explored:   iradix.New[*data.Value](),
		sequencers: &sync.Map{},
		paths:      &sync.Map{},
		settled:    &sync.Map{},
	}

	if client != nil {
		for prefix := range client.ListKeys("model/current") {
			value := client.Download(ctx, prefix)
			if value == nil {
				continue
			}

			var path string
			value.Lookup(&path, "path")
			if path == "" {
				continue
			}

			errnie.Debug("[model.NewModel]", "prefix", prefix, "path", path)

			model.tree, _, _ = model.tree.Insert([]byte(path), value)
		}
	}

	return model
}

// Evaluate processes one token for one symbol and returns the model's
// action, or nil when no action is taken.
//
// The token path accumulated per symbol is the trie key. The method
// handles three cases in order:
//  1. An open virtual whose band walk is over is settled and retired.
//  2. A path that matches the current policy tree acts on the stored role.
//  3. A new path at or above minPathLength starts a virtual exploration.
func (model *Model) Evaluate(value *data.Value) *data.Value {
	if value == nil {
		return nil
	}

	var token tokenizer.Token
	value.Lookup(&token, "token")

	if len(token.Chord) == 0 {
		return nil
	}

	symbol := value.Scope

	untyped, _ := model.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	sequencer := untyped.(*tokenizer.Sequencer)
	sequencer.Analyze([]tokenizer.Token{token})

	model.mu.Lock()
	path := model.appendPath(symbol, token)
	pathKey := path.String()
	model.mu.Unlock()

	// Publish the current token path to the dashboard every tick.
	model.publishMatch(symbol, path, "noop")

	// 1. Settle an open virtual whose band walk is over.
	if model.desk.Has(symbol, true) {
		virtual := model.desk.Virtual(symbol)
		if virtual != nil && virtual.ShouldExit() {
			model.desk.Exit(symbol, pathKey, true)
			model.Settle(virtual)
			model.resetPath(symbol)
			return nil
		}
	}

	// 2. Current policy: all branches lit up by the path must agree.
	if len(path) >= minPathLength {
		if val := prefixRole(model.tree, pathKey); val != nil {
			switch val.Role {
			case "enter":
				if !model.desk.Has(symbol, true) {
					model.desk.Enter(symbol, pathKey, true)
				}
				return val
			case "exit":
				if model.desk.Has(symbol, true) {
					virtual := model.desk.Virtual(symbol)
					model.desk.Exit(symbol, pathKey, true)
					if virtual != nil {
						model.Settle(virtual)
					}
					model.resetPath(symbol)
				}
				return val
			case "noop":
				return val
			}
		}
	}

	// A virtual is open and not done: nothing else to do this tick.
	if model.desk.Has(symbol, true) {
		return nil
	}

	// 3. Explore: the path must be long enough to be a meaningful pattern.
	if len(path) < minPathLength {
		return nil
	}

	// Already tracking this path (deepest prefix match).
	if val := prefixDeepest(model.explored, pathKey); val != nil {
		var score map[string]int
		val.Lookup(&score, "score")
		if score != nil && score["losses"] >= maxLosses {
			// Retired: too many losses, do not re-enter.
			return nil
		}
		// The virtual was settled but the path has not promoted yet:
		// re-enter to continue gathering evidence.
		model.desk.Enter(symbol, pathKey, true)
		return nil
	}

	// New path: start exploring.
	if err := model.desk.Enter(symbol, pathKey, true); err == nil {
		model.recordExplored(pathKey, symbol)
	}

	return nil
}

// Settle scores a closed virtual's outcome against its entry path in the
// explored tree, promotes the path into the current tree when it has
// enough wins, and retires the virtual from the desk.
//
// Idempotent: a position is settled at most once (tracked by ID).
// The position is not dropped from the desk so the UI can display it
// as a finished trade; it remains in the closed cache.
func (model *Model) Settle(position *broker.Position) {
	if position == nil || position.EntryPath == "" {
		return
	}

	if _, done := model.settled.LoadOrStore(position.ID, true); done {
		return
	}

	pathKey := position.EntryPath
	symbol := position.Symbol

	key := []byte(pathKey)
	val, found := model.explored.Get(key)

	if found {
		win := position.Return != nil && *position.Return > 0

		var score map[string]int
		val.Lookup(&score, "score")
		if score == nil {
			score = make(map[string]int)
		}

		if win {
			score["wins"]++
		} else {
			score["losses"]++
		}

		updated := data.NewValue("training", "explored", symbol)
		updated.Append(score, "score")
		model.explored, _, _ = model.explored.Insert(key, updated)

		if score["wins"] >= promotionWins && score["losses"] == 0 {
			model.promote(pathKey, symbol)
		}
	}

	// The position stays in the closed cache so the UI can display it
	// as a finished trade. It does not block new entries (Has checks
	// the open side only). The settled map above prevents re-scoring.
}

// Train inserts a value into the current policy tree and persists it.
// The value must carry a "path" layer with the token path string.
func (model *Model) Train(value *data.Value) {
	if value == nil {
		return
	}

	var path string
	value.Lookup(&path, "path")
	if path == "" {
		return
	}

	model.tree, _, _ = model.tree.Insert([]byte(path), value)

	if model.store != nil {
		model.store.WriteAtPrefix(value.Prefix, value.Bytes())
	}
}

// promote moves a path from the explored tree into the current policy
// tree and persists it.
func (model *Model) promote(pathKey, symbol string) {
	val := data.NewValue("training", "enter", symbol)
	val.Append(pathKey, "path")

	model.Train(val)
	model.explored, _, _ = model.explored.Delete([]byte(pathKey))
}

// recordExplored inserts a new path into the explored tree with a
// zeroed score.
func (model *Model) recordExplored(pathKey, symbol string) {
	val := data.NewValue("training", "explored", symbol)
	val.Append(map[string]int{"wins": 0, "losses": 0}, "score")
	model.explored, _, _ = model.explored.Insert([]byte(pathKey), val)
}

// appendPath adds a token to the symbol's accumulated path, trimming
// to maxPathLength, and stores it. Caller must hold model.mu.
func (model *Model) appendPath(symbol string, token tokenizer.Token) tokenizer.Tokens {
	untyped, ok := model.paths.Load(symbol)
	var path tokenizer.Tokens
	if ok {
		path = untyped.(tokenizer.Tokens)
	}
	path = append(path, token)

	if len(path) > maxPathLength {
		path = path[len(path)-maxPathLength:]
	}

	model.paths.Store(symbol, path)
	return path
}

// resetPath clears the symbol's accumulated path. Caller must hold model.mu.
func (model *Model) resetPath(symbol string) {
	model.paths.Store(symbol, tokenizer.Tokens{})
}

// publishMatch sends the symbol's current token path to the dashboard as a
// cognition/match frame so the position card's token ribbon and strength
// gauge render live.
func (model *Model) publishMatch(symbol string, path tokenizer.Tokens, action string) {
	if model.hub == nil {
		return
	}

	tokens := make([]string, 0, len(path))
	for _, t := range path {
		tokens = append(tokens, t.String())
	}

	match := map[string]any{
		"symbol": symbol,
		"tick":   data.Tick.Load(),
		"tokens": tokens,
		"stored": false,
		"match":  path.String(),
		"depth":  len(path),
		"action": action,
	}

	value := data.NewValue("cognition", "match", symbol)
	value.Append(match, "cognition")

	io.Copy(model.hub, value)
}

// prefixRole returns the value whose role is shared by every branch lit
// up by key, or nil when there are no matches or the branches disagree.
func prefixRole(tree *iradix.Tree[*data.Value], key string) *data.Value {
	it := tree.Root().Iterator()
	it.SeekPrefix([]byte(key))

	var role string
	var val *data.Value
	var found bool

	for {
		_, v, ok := it.Next()
		if !ok {
			break
		}
		if !found {
			role = v.Role
			val = v
			found = true
			continue
		}
		if v.Role != role {
			return nil
		}
	}

	return val
}

// prefixDeepest returns the longest-key match lit up by key, or nil.
func prefixDeepest(tree *iradix.Tree[*data.Value], key string) *data.Value {
	it := tree.Root().Iterator()
	it.SeekPrefix([]byte(key))

	var val *data.Value
	for {
		_, v, ok := it.Next()
		if !ok {
			break
		}
		val = v
	}

	return val
}
