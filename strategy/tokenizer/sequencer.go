package tokenizer

import (
	"sort"
	"strings"
)

/*
Sequitur is the per-symbol grammar engine. It receives one tick's region
tokens as a single chord, feeds that chord to the Sequitur grammar, and
emits one regime token when a repeated digram is replaced by a rule.

That replacement is the sentence boundary: two occurrences of the same
regions are a shorter code than spelling them twice. The emitted token is
one trie edge, the rule's terminals, not a step per region and not a cut
taken from the bytes of the region label.

The Sequencer is the bridge between the grid's raw observations and the
model's trie path. It owns the grammar. The byte-level MDL stays available
for calibration; it is not the regime cut.
*/
type Sequencer struct {
	sequitur *Sequitur
	mdl      *MDL
	// pending accumulates region terminals since the last rule.
	pending []string
	// pendingBrightness is the brightest region seen while pending.
	pendingBrightness float64
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
Analyze processes one tick's region tokens as one chord. The regions are
the ones the grid kept at or above that symbol's brightness midpoint.

A repeated chord becomes a grammar rule. That rule is one regime token:
its terminals, in order, as a single trie edge. A chord that has not
repeated returns a zero Token. Callers treat a zero Token as "this tick
is inside a regime that has not closed."
*/
func (sequencer *Sequencer) Analyze(regionTokens []Token) Token {
	if len(regionTokens) == 0 {
		return Token{}
	}

	symbols := make([]string, 0, len(regionTokens))
	var brightness float64

	for _, region := range regionTokens {
		if region.Brightness > brightness {
			brightness = region.Brightness
		}

		for _, sym := range region.Symbols {
			symbols = append(symbols, sym)
		}
	}

	if len(symbols) == 0 {
		return Token{}
	}

	sort.Strings(symbols)
	deduped := symbols[:0]

	for i, sym := range symbols {
		if i == 0 || sym != symbols[i-1] {
			deduped = append(deduped, sym)
		}
	}

	sequencer.pending = append(sequencer.pending, deduped...)

	if brightness > sequencer.pendingBrightness {
		sequencer.pendingBrightness = brightness
	}

	rule, boundary := sequencer.sequitur.Analyze(deduped)

	if !boundary || rule == nil {
		return Token{}
	}

	terminals := sequencer.sequitur.TerminalNames(rule)

	if len(terminals) == 0 {
		return Token{}
	}

	sequencer.pending = nil
	sequencer.pendingBrightness = 0

	if brightness < 0 {
		brightness = 0
	}

	if brightness > 1 {
		brightness = 1
	}

	return NewToken(terminals, brightness, brightness, 0)
}

/*
Flush emits region terminals accumulated since the last rule. The model
calls it when a sentence ends so a trailing chord is not left unnamed.
A zero Token means nothing was pending.
*/
func (sequencer *Sequencer) Flush() Token {
	if len(sequencer.pending) == 0 {
		return Token{}
	}

	symbols := sequencer.pending
	brightness := sequencer.pendingBrightness
	sequencer.pending = nil
	sequencer.pendingBrightness = 0

	return NewToken(symbols, brightness, brightness, 0)
}

/*
Grammar exposes the underlying Sequitur for persistence and the dashboard.
*/
func (sequencer *Sequencer) Grammar() *Sequitur {
	return sequencer.sequitur
}

/*
MDL exposes the boundary detector for calibration feedback.
*/
func (sequencer *Sequencer) MDL() *MDL {
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
