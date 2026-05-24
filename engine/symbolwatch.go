package engine

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
)

// TODO: REMOVE

const symbolActivityHalfLife = 30 * time.Second

/*
SymbolWatch tracks recent market activity and yields prioritized scan sets.
Hot symbols are evaluated every tick; cold symbols rotate through the budget.
*/
type SymbolWatch struct {
	mu       sync.RWMutex
	all      []string
	activity map[string]float64
	dirty    map[string]struct{}
	rotate   int
}

/*
NewSymbolWatch indexes the tradable symbol universe.
*/
func NewSymbolWatch(symbols []string) *SymbolWatch {
	activity := make(map[string]float64, len(symbols))

	for _, symbol := range symbols {
		activity[symbol] = 0
	}

	return &SymbolWatch{
		all:      append([]string(nil), symbols...),
		activity: activity,
		dirty:    make(map[string]struct{}),
	}
}

/*
NoteTrade marks a symbol active after executed volume arrives.
*/
func (symbolWatch *SymbolWatch) NoteTrade(symbol string, volume float64) {
	if symbol == "" || volume <= 0 {
		return
	}

	symbolWatch.mu.Lock()
	defer symbolWatch.mu.Unlock()

	symbolWatch.activity[symbol] += volume
	symbolWatch.dirty[symbol] = struct{}{}
}

/*
NoteBook marks a symbol active after an order-book update.
*/
func (symbolWatch *SymbolWatch) NoteBook(symbol string) {
	if symbol == "" {
		return
	}

	symbolWatch.mu.Lock()
	defer symbolWatch.mu.Unlock()

	symbolWatch.activity[symbol] += 1
	symbolWatch.dirty[symbol] = struct{}{}
}

/*
NoteTicker marks a symbol active from quote movement.
*/
func (symbolWatch *SymbolWatch) NoteTicker(symbol string, changePct float64) {
	if symbol == "" {
		return
	}

	symbolWatch.mu.Lock()
	defer symbolWatch.mu.Unlock()

	symbolWatch.activity[symbol] += math.Abs(changePct)
	symbolWatch.dirty[symbol] = struct{}{}
}

/*
Decay fades activity scores and clears dirty flags for the next scan tick.
*/
func (symbolWatch *SymbolWatch) Decay(now time.Time) {
	_ = now

	halfLife := symbolActivityHalfLife

	if config.System.SymbolActivityHalfLife > 0 {
		halfLife = config.System.SymbolActivityHalfLife
	}

	factor := math.Exp(-config.System.RescoreEvery.Seconds() / halfLife.Seconds())

	symbolWatch.mu.Lock()
	defer symbolWatch.mu.Unlock()

	for symbol, score := range symbolWatch.activity {
		symbolWatch.activity[symbol] = score * factor
	}

	symbolWatch.dirty = make(map[string]struct{})
}

/*
ScanSet returns symbols to evaluate this tick: dirty first, then hottest, then rotation.
*/
func (symbolWatch *SymbolWatch) ScanSet(budget int) []string {
	symbolWatch.mu.RLock()
	defer symbolWatch.mu.RUnlock()

	if budget <= 0 || budget >= len(symbolWatch.all) {
		return append([]string(nil), symbolWatch.all...)
	}

	selected := make([]string, 0, budget)
	seen := make(map[string]struct{}, budget)

	dirtySymbols := make([]string, 0, len(symbolWatch.dirty))

	for symbol := range symbolWatch.dirty {
		dirtySymbols = append(dirtySymbols, symbol)
	}

	sort.Slice(dirtySymbols, func(left, right int) bool {
		return symbolWatch.activity[dirtySymbols[left]] > symbolWatch.activity[dirtySymbols[right]]
	})

	for _, symbol := range dirtySymbols {
		if len(selected) >= budget {
			return selected
		}

		selected = append(selected, symbol)
		seen[symbol] = struct{}{}
	}

	ranked := make([]string, 0, len(symbolWatch.all))
	cold := make([]string, 0, len(symbolWatch.all))

	for _, symbol := range symbolWatch.all {
		if _, ok := seen[symbol]; ok {
			continue
		}

		if symbolWatch.activity[symbol] > 0 {
			ranked = append(ranked, symbol)
			continue
		}

		cold = append(cold, symbol)
	}

	sort.Slice(ranked, func(left, right int) bool {
		leftScore := symbolWatch.activity[ranked[left]]
		rightScore := symbolWatch.activity[ranked[right]]

		if leftScore != rightScore {
			return leftScore > rightScore
		}

		return ranked[left] < ranked[right]
	})

	for _, symbol := range ranked {
		if len(selected) >= budget {
			return selected
		}

		selected = append(selected, symbol)
		seen[symbol] = struct{}{}
	}

	if len(cold) == 0 {
		return selected
	}

	for step := 0; step < len(cold) && len(selected) < budget; step++ {
		index := (symbolWatch.rotate + step) % len(cold)
		symbol := cold[index]

		if _, ok := seen[symbol]; ok {
			continue
		}

		selected = append(selected, symbol)
		seen[symbol] = struct{}{}
	}

	return selected
}

/*
AdvanceRotation moves the cold-symbol round-robin cursor.
*/
func (symbolWatch *SymbolWatch) AdvanceRotation(budget int) {
	symbolWatch.mu.Lock()
	defer symbolWatch.mu.Unlock()

	if len(symbolWatch.all) == 0 {
		return
	}

	step := budget

	if step <= 0 {
		step = len(symbolWatch.all)
	}

	symbolWatch.rotate = (symbolWatch.rotate + step) % len(symbolWatch.all)
}
