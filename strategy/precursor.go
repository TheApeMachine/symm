package strategy

import (
	"encoding/binary"
	"hash/fnv"
	"iter"
	"slices"
	"sync"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

/*
Precursor encodes a market's temporal Impulse sequence and position state
into a radix trie key. A precursor is a temporal tape fragment:
I₀ → I₁ → I₂ → ... → Iₙ
preserving temporal ordering, symbol scope, and holding scope without
flattening time or repeating identical impulse states.
*/
type Precursor struct {
	*core.PrimitiveError
	holding   func(symbol string) bool
	key       []byte
	histories map[string][]uint64
	mu        sync.RWMutex
	question  cognition.Question
	command   cognition.Command
}

func NewPrecursor() *Precursor {
	return &Precursor{
		PrimitiveError: core.NewPrimitiveError(),
		histories:      make(map[string][]uint64),
	}
}

func (precursor *Precursor) SetHolding(holding func(symbol string) bool) {
	precursor.holding = holding
}

func scopeToken(symbol string, holding bool) uint64 {
	hasher := fnv.New64a()
	hasher.Write([]byte(symbol))
	val := hasher.Sum64()

	if holding {
		val ^= 0x5555555555555555
	}

	return val
}

/*
ImpulseToken fingerprints one Impulse state from its active region conditions.
Two Impulse states are identical if and only if their region conditions match.
*/
func ImpulseToken(regions []grid.Region) uint64 {
	if len(regions) == 0 {
		return 0
	}

	hasher := fnv.New64a()
	var buf [8]byte

	for _, region := range regions {
		binary.BigEndian.PutUint64(buf[:], region.Condition)
		hasher.Write(buf[:])
	}

	return hasher.Sum64()
}

/*
Reset clears the temporal sequence for a symbol (e.g. at structural boundaries).
*/
func (precursor *Precursor) Reset(symbol string) {
	precursor.mu.Lock()
	defer precursor.mu.Unlock()
	delete(precursor.histories, symbol)
}

/*
Tokens returns a copy of the temporal token sequence for a symbol.
*/
func (precursor *Precursor) Tokens(symbol string) []uint64 {
	precursor.mu.RLock()
	defer precursor.mu.RUnlock()
	history := precursor.histories[symbol]

	if len(history) == 0 {
		return nil
	}

	return slices.Clone(history)
}

/*
SetTokens sets the temporal token sequence for a symbol.
*/
func (precursor *Precursor) SetTokens(symbol string, tokens []uint64) {
	precursor.mu.Lock()
	defer precursor.mu.Unlock()

	if len(tokens) == 0 {
		delete(precursor.histories, symbol)
		return
	}

	precursor.histories[symbol] = slices.Clone(tokens)
}

/*
Step advances the temporal precursor for a symbol with a new Impulse state.
Repeated identical Impulse states (scheduler frequency envelopes without
meaningful development) do not advance the sequence (Rule 8).
Returns true if a new distinct state was appended.
*/
func (precursor *Precursor) Step(impulse *grid.Impulse) bool {
	if impulse == nil || !impulse.Ready || len(impulse.Regions) == 0 {
		return false
	}

	tok := ImpulseToken(impulse.Regions)

	if tok == 0 {
		return false
	}

	precursor.mu.Lock()
	defer precursor.mu.Unlock()

	history := precursor.histories[impulse.Label]

	if len(history) > 0 && history[len(history)-1] == tok {
		return false
	}

	precursor.histories[impulse.Label] = append(history, tok)
	return true
}

/*
EncodeTokens builds a trie key from a scope token and a temporal sequence of impulse tokens:
[scopeToken, I₀, I₁, ..., Iₙ]
*/
func EncodeTokens(symbol string, holding bool, tokens []uint64) []byte {
	if len(tokens) == 0 {
		return nil
	}

	key := make([]byte, (1+len(tokens))*8)
	binary.BigEndian.PutUint64(key[0:8], scopeToken(symbol, holding))

	for index, tok := range tokens {
		binary.BigEndian.PutUint64(key[8+index*8:], tok)
	}

	return key
}

/*
Key returns the encoded key for the current temporal sequence of the symbol.
*/
func (precursor *Precursor) Key(symbol string, holding bool) []byte {
	precursor.mu.RLock()
	tokens := precursor.histories[symbol]

	if len(tokens) == 0 {
		precursor.mu.RUnlock()
		return nil
	}

	key := EncodeTokens(symbol, holding, tokens)
	precursor.mu.RUnlock()
	return key
}

/*
Encode steps the impulse state into the temporal sequence (filtering duplicates)
and returns the current temporal precursor key.
*/
func (precursor *Precursor) Encode(impulse *grid.Impulse, holding bool) []byte {
	if impulse == nil || !impulse.Ready || len(impulse.Regions) == 0 {
		return nil
	}

	precursor.Step(impulse)
	return precursor.Key(impulse.Label, holding)
}

func (precursor *Precursor) Next(input iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range input {
			impulse := (*grid.Impulse)(arriving)
			held := false

			if precursor.holding != nil && impulse != nil {
				held = precursor.holding(impulse.Label)
			}

			contextKey := precursor.Encode(impulse, held)
			precursor.question = cognition.Question{
				Context: contextKey,
				Exact:   false,
			}
			precursor.command = cognition.Command{Evaluate: &precursor.question}

			if !yield(unsafe.Pointer(&precursor.command)) {
				return
			}
		}
	}
}
