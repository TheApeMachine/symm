package strategy

import (
	"strings"
	"sync"

	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
)

/*
Path is the per-symbol accumulation of cut tokens (trie edges). While flat,
it grows the enter prefix; Reset on fill clears it so holding ticks build a
separate exit prefix (stored on the position as entry_path vs exit_path).

The training node runs alone in the workspace strategy ring on one disruptor
handler goroutine, so path state needs no locking.
*/
type Path struct {
	mu              sync.Mutex
	bySymbol        map[string]tokenizer.Tokens
	triggerBySymbol map[string]tokenizer.Tokens
	limit           int
}

func NewPath() *Path {
	return &Path{
		bySymbol:        make(map[string]tokenizer.Tokens),
		triggerBySymbol: make(map[string]tokenizer.Tokens),
		limit:           system.Cfg.Learning.MaxPathLength,
	}
}

func (path *Path) Append(symbol string, token tokenizer.Token) tokenizer.Tokens {
	path.mu.Lock()
	defer path.mu.Unlock()

	if token.String() == "" {
		return path.bySymbol[symbol]
	}

	tokens := path.bySymbol[symbol]

	if len(tokens) > 0 && tokens[len(tokens)-1].String() == token.String() {
		return tokens
	}

	tokens = append(tokens, token)

	if path.limit > 0 && len(tokens) > path.limit {
		tokens = tokens[len(tokens)-path.limit:]
	}

	path.bySymbol[symbol] = tokens
	return tokens
}

func (path *Path) Reset(symbol string) {
	path.mu.Lock()
	defer path.mu.Unlock()

	path.bySymbol[symbol] = nil
	path.triggerBySymbol[symbol] = nil
}

func (path *Path) AppendTrigger(symbol string, token tokenizer.Token) tokenizer.Tokens {
	path.mu.Lock()
	defer path.mu.Unlock()

	if token.String() == "" {
		return path.triggerBySymbol[symbol]
	}

	tokens := path.triggerBySymbol[symbol]

	if len(tokens) > 0 && tokens[len(tokens)-1].String() == token.String() {
		return tokens
	}

	tokens = append(tokens, token)

	if path.limit > 0 && len(tokens) > path.limit {
		tokens = tokens[len(tokens)-path.limit:]
	}

	path.triggerBySymbol[symbol] = tokens
	return tokens
}

func (path *Path) HasTrigger(symbol string) bool {
	path.mu.Lock()
	defer path.mu.Unlock()

	return len(path.triggerBySymbol[symbol]) > 0
}

// TriggerLive is the exit precursor prefix used for policy keys and explore storage.
func (path *Path) TriggerLive(symbol string) string {
	path.mu.Lock()
	defer path.mu.Unlock()

	return path.triggerBySymbol[symbol].String()
}

// PolicyLive is the trie prefix for this phase (enter path or exit trigger path).
func (path *Path) PolicyLive(symbol string, enter bool) string {
	path.mu.Lock()
	defer path.mu.Unlock()

	if enter {
		return path.bySymbol[symbol].String()
	}

	return path.triggerBySymbol[symbol].String()
}

func (path *Path) Len(symbol string) int {
	path.mu.Lock()
	defer path.mu.Unlock()

	return len(path.bySymbol[symbol])
}

func (path *Path) AtLimit(symbol string) bool {
	return path.limit > 0 && path.Len(symbol) >= path.limit
}

// Live is the symbol's accumulated path key before this tick's append.
func (path *Path) Live(symbol string) string {
	path.mu.Lock()
	defer path.mu.Unlock()

	return path.bySymbol[symbol].String()
}

// Follow replaces the current sentence with the grammar spelling.
func (path *Path) Follow(symbol string, enter bool, spell []string) string {
	if path == nil {
		return strings.Join(spell, "/")
	}

	path.mu.Lock()
	defer path.mu.Unlock()

	tokens := make(tokenizer.Tokens, 0, len(spell))
	for _, label := range spell {
		if label == "" {
			continue
		}
		tokens = append(tokens, tokenizer.NewToken([]string{label}, 0, 0))
	}
	if path.limit > 0 && len(tokens) > path.limit {
		tokens = tokens[len(tokens)-path.limit:]
	}

	if enter {
		path.bySymbol[symbol] = tokens
	} else {
		path.triggerBySymbol[symbol] = tokens
	}
	return tokens.String()
}

// After is live plus one token edge (trie lookup key for this step).
func (path *Path) After(live string, token tokenizer.Token) string {
	label := token.String()
	if label == "" {
		return live
	}
	if live == "" {
		return label
	}
	if live == label || strings.HasSuffix(live, "/"+label) {
		return live
	}
	return live + "/" + label
}
