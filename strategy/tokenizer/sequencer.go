package tokenizer

import (
	"sort"
	"strings"
)

/*
Sequitur is the per-symbol grammar engine. It receives one tick's region
tokens (individual, not pre-grouped), feeds them to the Sequitur grammar,
then feeds the resulting grammar symbols to MDL for boundary detection.

A token for the model is a variable-length cut over the grammar stream:
the MDL decides where to cut, and the symbols accumulated since the last
cut form one Token. The token is NOT a single region — it is a structural
segment of the market tape.

The Sequencer is the bridge between the grid's raw observations and the
model's trie path. It owns both the grammar and the boundary detector.
*/
type Sequencer struct {
	sequitur *Sequitur
	mdl      *MDL
	// pending accumulates grammar symbols since the last MDL boundary.
	pending []string
	// pendingBrightness is the aggregate brightness for symbols in pending.
	pendingBrightness float64
	pendingCount      int
}

// NewSequitur is defined in sequitur.go.

/*
NewSequencer creates a Sequencer with a fresh grammar and MDL boundary
detector. Each symbol in the market gets its own Sequencer instance.
*/
func NewSequencer() *Sequencer {
	return &Sequencer{
		sequitur: NewSequitur(),
		mdl:      NewMDL(NewCalibrator()),
	}
}

/*
Analyze processes one tick's region tokens. Each region is an individual
token (R01, R02, …) that the grid emitted above the brightness midpoint.

The method feeds all tokens to the grammar, then feeds the resulting
grammar symbols to MDL. If MDL detects a boundary, it emits the
accumulated symbols as a Token. Otherwise it returns a zero Token (no
cut this tick — the symbols remain pending for the next boundary).

Callers should check `token.Symbols != nil` to determine if a token was
produced. A zero Token means "no boundary this tick, keep accumulating."
*/
func (sequencer *Sequitur) Analyze(regionTokens []Token) Token {
	if len(regionTokens) == 0 {
		return Token{}
	}

	// 1. Feed individual region keys to the grammar.
	symbols := make([]string, 0, len(regionTokens))
	for _, rt := range regionTokens {
		for _, sym := range rt.Symbols {
			symbols = append(symbols, sym)
		}
	}

	if len(symbols) == 0 {
		return Token{}
	}

	// Deduplicate and sort within the tick: the same region appearing
	// in two signals is one symbol, not two.
	sort.Strings(symbols)
	deduped := symbols[:0]
	for i, s := range symbols {
		if i == 0 || s != symbols[i-1] {
			deduped = append(deduped, s)
		}
	}

	// Feed to the grammar.
	sequencer.sequitur.Analyze(deduped)

	// 2. Get the grammar's output symbols for this tick's positions.
	// After Analyze, the root's tail holds the grammar's encoding.
	outputSymbols := sequencer.sequitur.Tail(len(deduped))
	if len(outputSymbols) == 0 {
		outputSymbols = deduped
	}

	// 3. Feed each output symbol's bytes to MDL.
	var aggregateBrightness float64
	for _, rt := range regionTokens {
		aggregateBrightness = max(aggregateBrightness, rt.Brightness)
	}

	var emitted bool
	for _, sym := range outputSymbols {
		for _, b := range []byte(sym) {
			isBoundary, _, _, meta := sequencer.mdl.Analyze(0, b)
			if isBoundary && !emitted {
				// MDL says: cut here. Emit what we've accumulated.
				sequencer.pending = append(sequencer.pending, sym)
				sequencer.pendingBrightness = aggregateBrightness
				sequencer.pendingCount++
				return sequencer.emitToken(meta)
			}
			if !isBoundary {
				// No boundary yet: accumulate this symbol.
				sequencer.pending = append(sequencer.pending, sym)
				sequencer.pendingBrightness += aggregateBrightness
				sequencer.pendingCount++
			}
		}
	}

	return Token{}
}

/*
emitToken constructs a Token from the accumulated pending symbols and
resets the pending buffer. The confidence is derived from the MDL's
boundary gain (how strong the structural break was) modulated by the
aggregate brightness.
*/
func (sequencer *Sequitur) emitToken(meta SegmentMeta) Token {
	symbols := sequencer.pending
	sequencer.pending = nil
	sequencer.pendingBrightness = 0
	sequencer.pendingCount = 0

	if len(symbols) == 0 {
		return Token{}
	}

	// Confidence: the MDL's last boundary gain (0..1 scale) blended with
	// the brightness of the regions that contributed. A strong structural
	// cut in a high-brightness regime is high confidence; a weak cut in a
	// low-brightness regime is low confidence.
	boundaryStrength := sequencer.mdl.LastGain()
	brightness := sequencer.pendingBrightness / float64(max(sequencer.pendingCount, 1))

	confidence := 0.6*boundaryStrength + 0.4*brightness
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.0 {
		confidence = 0.0
	}

	return NewToken(symbols, confidence, brightness, meta)
}

/*
Flush forces emission of any pending symbols as a Token. Called when the
model resets its path (after a settle) to avoid losing accumulated grammar
structure. Returns a zero Token if nothing is pending.
*/
func (sequencer *Sequitur) Flush() Token {
	if len(sequencer.pending) == 0 {
		return Token{}
	}

	symbols := sequencer.pending
	sequencer.pending = nil
	brightness := sequencer.pendingBrightness / float64(max(sequencer.pendingCount, 1))
	sequencer.pendingBrightness = 0
	sequencer.pendingCount = 0

	return NewToken(symbols, brightness, brightness, 0)
}

/*
Grammar exposes the underlying Sequitur for persistence and the dashboard.
*/
func (sequencer *Sequitur) Grammar() *Sequitur {
	return sequencer.sequitur
}

/*
MDL exposes the boundary detector for calibration feedback.
*/
func (sequencer *Sequitur) MDL() *MDL {
	return sequencer.mdl
}

/*
Chord is the legacy convenience: the tick's region set as one sorted,
deduplicated bracketed string. Retained for the dashboard's GatePanel
and for the "fired" field in the SequiturFrame. Not used for model
tokens — the model's tokens come from MDL cuts.
*/
func Chord(tokens []Token) string {
	seen := make(map[string]bool, len(tokens))
	keys := make([]string, 0, len(tokens))

	for _, token := range tokens {
		for _, s := range token.Symbols {
			if s == "" || seen[s] {
				continue
			}
			seen[s] = true
			keys = append(keys, s)
		}
	}

	if len(keys) == 0 {
		return ""
	}

	sort.Strings(keys)

	if len(keys) == 1 {
		return keys[0]
	}
	return "[" + strings.Join(keys, "-") + "]"
}
