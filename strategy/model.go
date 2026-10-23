package strategy

import (
	"context"
	"io"
	"strings"
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

	// maxThreshold is the ceiling: a path whose adaptive threshold
	// reaches this value can no longer be entered and is removed.
	maxThreshold = 1.0

	// thresholdDecay is the multiplicative factor applied on a win.
	// The threshold decreases (path becomes easier to enter).
	thresholdDecay = 0.7

	// thresholdGrowth is the multiplicative factor applied on a loss.
	// The threshold increases (path becomes harder to enter).
	thresholdGrowth = 1.5

	// promotionWins is the minimum number of observations required
	// before a path can promote to the current policy tree.
	promotionWins = 3

	// promotionWinRate is the minimum win rate (wins / total) required
	// for promotion. A path that has 3+ observations and a win rate
	// above this promotes.
	promotionWinRate = 0.5

	// demotionLosses is the number of consecutive losses in the current
	// tree that demotes a path back to explored.
	demotionLosses = 3
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

/*
pathScore tracks the adaptive state of an explored path. The threshold
is the entry gate: a token must have confidence >= threshold to trigger
the path. Wins decrease the threshold (easier to enter); losses increase
it (harder to enter). At maxThreshold the path is retired.
*/
type pathScore struct {
	Threshold float64 `json:"threshold"`
	Wins      int     `json:"wins"`
	Losses    int     `json:"losses"`
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
		for prefix := range client.ListKeys("policy/current") {
			// Only load enter/exit keys, not noop.
			if !strings.Contains(prefix, "/enter") && !strings.Contains(prefix, "/exit") {
				continue
			}
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

/*
Evaluate processes one token for one symbol and returns the model's
action, or nil when no action is taken.

The token path accumulated per symbol is the trie key. The method
handles three cases in order:
 1. An open virtual whose band walk is over is settled and retired.
 2. A path that matches the current policy tree acts on the stored role.
 3. A new path at or above minPathLength starts a virtual exploration,
    gated by the path's adaptive threshold.
*/
func (model *Model) Evaluate(symbol string, token tokenizer.Token) *data.Value {
	if len(token.Symbols) == 0 {
		return nil
	}

	// Feed the token to the symbol's sequencer for grammar/MDL tracking.
	// The sequencer's output is not the model's decision input — the model
	// uses the token directly. The sequencer exists for the dashboard's
	// grammar viz and for future use as a confidence modulator.
	untyped, _ := model.sequencers.LoadOrStore(symbol, tokenizer.NewSequencer())
	sequencer := untyped.(*tokenizer.Sequencer)
	sequencer.Analyze([]tokenizer.Token{token})

	// Accumulate the token into the symbol's path.
	model.mu.Lock()
	path := model.appendPath(symbol, token)
	pathKey := path.String()
	model.mu.Unlock()

	// Publish the current token path to the dashboard.
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

	// 2. Current policy: S3 prefix walk — find all stored paths that are
	//    prefixes of the current path. All must agree on action.
	if len(path) >= minPathLength {
		action, matchDepth := model.evaluateTree(pathKey)
		switch action {
		case "enter":
			if !model.desk.Has(symbol, true) {
				model.desk.Enter(symbol, pathKey, true)
			}
			val := data.NewValue("training", "enter", symbol)
			val.Append(pathKey, "path")
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
			val := data.NewValue("training", "exit", symbol)
			val.Append(pathKey, "path")
			return val
		case "noop":
			// Dampener: the path matches the tree but the action is noop.
			// This is intentional — early trie positions default to noop
			// to prevent premature entries.
			val := data.NewValue("training", "noop", symbol)
			val.Append(pathKey, "path")
			return val
		}
	}

	// A virtual is open and not done: nothing else to do this tick.
	if model.desk.Has(symbol, true) {
		return nil
	}

	// 3. Explore: the path must be long enough and the token's confidence
	//    must exceed the path's adaptive threshold.
	if len(path) < minPathLength {
		return nil
	}

	// Already tracking this path: check its threshold.
	if val := model.explored.Get([]byte(pathKey)); val != nil {
		var score pathScore
		val.Lookup(&score, "score")

		// Retired: threshold at max, do not re-enter.
		if score.Threshold >= maxThreshold {
			return nil
		}

		// Confidence gate: the token must be confident enough.
		if token.Confidence < score.Threshold {
			return nil
		}

		// Re-enter to continue gathering evidence.
		model.desk.Enter(symbol, pathKey, true)
		return nil
	}

	// New path: start exploring. The initial threshold is the token's
	// confidence — a high-confidence first observation sets a high bar
	// for subsequent entries (conservative), while a low-confidence one
	// sets a low bar (the path is easy to trigger, will learn fast).
	if err := model.desk.Enter(symbol, pathKey, true); err == nil {
		model.recordExplored(pathKey, symbol, token.Confidence)
	}

	return nil
}

/*
evaluateTree performs the S3-prefix walk: it checks each prefix of the
given path (1 segment, 2 segments, …, N segments) against the current
policy tree. All matching stored values must agree on the same action
for the action to be returned. Disagreement yields "noop" (the dampener).
No matches yields ("", 0).
*/
func (model *Model) evaluateTree(pathKey string) (string, int) {
	segments := strings.Split(pathKey, "/")

	var action string
	var found bool
	var matchDepth int

	for i := 1; i <= len(segments); i++ {
		prefix := strings.Join(segments[:i], "/")
		val, ok := model.tree.Get([]byte(prefix))
		if !ok {
			continue
		}

		if !found {
			action = val.Role
			found = true
			matchDepth = i
			continue
		}

		if val.Role != action {
			// Disagreement: dampen with noop.
			return "noop", matchDepth
		}
		matchDepth = i
	}

	if !found {
		return "", 0
	}

	return action, matchDepth
}

/*
Settle scores a closed virtual's outcome against its entry path in the
explored tree, adjusts the adaptive threshold, promotes the path into
the current tree when it has enough wins at a sufficient win rate, and
demotes current-tree paths that keep losing.

Idempotent: a position is settled at most once (tracked by ID).
*/
func (model *Model) Settle(position *broker.Position) {
	if position == nil || position.EntryPath == "" {
		return
	}

	if _, done := model.settled.LoadOrStore(position.ID, true); done {
		return
	}

	pathKey := position.EntryPath
	symbol := position.Symbol

	win := position.Return != nil && *position.Return > 0

	// Update the explored tree's adaptive threshold.
	if val, found := model.explored.Get([]byte(pathKey)); found {
		var score pathScore
		val.Lookup(&score, "score")

		if win {
			score.Wins++
			score.Threshold *= thresholdDecay
		} else {
			score.Losses++
			score.Threshold *= thresholdGrowth
			if score.Threshold > maxThreshold {
				score.Threshold = maxThreshold
			}
		}

		updated := data.NewValue("training", "explored", symbol)
		updated.Append(score, "score")

		model.explored, _, _ = model.explored.Insert([]byte(pathKey), updated)

		// Retire: threshold at max means the path is no longer enterable.
		if score.Threshold >= maxThreshold {
			model.explored, _, _ = model.explored.Delete([]byte(pathKey))
			return
		}

		// Promote: enough observations and sufficient win rate.
		total := score.Wins + score.Losses
		if total >= promotionWins {
			winRate := float64(score.Wins) / float64(total)
			if winRate > promotionWinRate {
				model.promote(pathKey, symbol)
			}
		}
	}

	// Demotion check: if the path is in the current tree and just lost,
	// track consecutive losses. At demotionLosses, remove from current.
	if val, found := model.tree.Get([]byte(pathKey)); found {
		if !win {
			var losses int
			val.Lookup(&losses, "losses")
			losses++

			if losses >= demotionLosses {
				// Demote: move back to explored with elevated threshold.
				model.tree, _, _ = model.tree.Delete([]byte(pathKey))

				exploredVal := data.NewValue("training", "explored", symbol)
				exploredVal.Append(pathScore{
					Threshold: 0.8,
					Wins:      0,
					Losses:    losses,
				}, "score")
				model.explored, _, _ = model.explored.Insert([]byte(pathKey), exploredVal)
			} else {
				// Update the loss counter on the current tree entry.
				updated := data.NewValue("training", val.Role, symbol)
				updated.Append(pathKey, "path")
				updated.Append(losses, "losses")
				model.tree, _, _ = model.tree.Insert([]byte(pathKey), updated)
			}
		} else {
			// Win: reset the loss counter.
			updated := data.NewValue("training", val.Role, symbol)
			updated.Append(pathKey, "path")
			updated.Append(0, "losses")
			model.tree, _, _ = model.tree.Insert([]byte(pathKey), updated)
		}
	}
}

/*
Train inserts a value into the current policy tree and persists it.
The value must carry a "path" layer with the token path string.
*/
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
		action := value.Role
		if action == "" {
			action = "enter"
		}
		storeKey := "policy/current/" + path + "/" + action + ".bin"
		value.Prefix = storeKey
		model.store.WriteAtPrefix(storeKey, value.Bytes())
	}
}

/*
promote moves a path from the explored tree into the current policy tree
and persists it.
*/
func (model *Model) promote(pathKey, symbol string) {
	val := data.NewValue("training", "enter", symbol)
	val.Append(pathKey, "path")
	val.Append(0, "losses")

	model.Train(val)
	model.explored, _, _ = model.explored.Delete([]byte(pathKey))
}

/*
recordExplored inserts a new path into the explored tree with the
token's confidence as the initial threshold.
*/
func (model *Model) recordExplored(pathKey, symbol string, confidence float64) {
	score := pathScore{
		Threshold: confidence,
		Wins:      0,
		Losses:    0,
	}

	val := data.NewValue("training", "explored", symbol)
	val.Append(pathKey, "path")
	val.Append(score, "score")
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

/*
publishMatch sends the symbol's current token path to the dashboard as a
cognition/match frame so the position card's token ribbon and strength
gauge render live. The shape matches the frontend's CognitionMatch type.
*/
func (model *Model) publishMatch(symbol string, path tokenizer.Tokens, action string) {
	if model.hub == nil {
		return
	}

	match := map[string]any{
		"symbol": symbol,
		"tick":   data.Tick.Load(),
		"tokens": path.StringList(),
		"stored": false,
		"match":  path.String(),
		"depth":  len(path),
		"action": action,
	}

	value := data.NewValue("cognition", "match", symbol)
	value.Append(match, "cognition")

	io.Copy(model.hub, value)
}
