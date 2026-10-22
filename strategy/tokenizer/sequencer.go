package tokenizer

import (
	"sort"
	"strings"
)

/*
Sequencer turns each tick's Tokens (one per observed region, all produced at
the same instant) into one chord symbol — the bracketed set of region keys,
[R01-R02-R03] — and feeds that single symbol per tick into Sequitur. Grammar
structure therefore spans ticks instead of inventing order between regions
that co-occurred. MDL makes the cut over the chord identities as a byte
stream, exactly as it did over raw bytes.
*/
type Sequencer struct {
	mdl      *MDL
	sequitur *Sequitur
	lastRule *Rule
}

func NewSequencer() *Sequencer {
	return &Sequencer{
		mdl:      NewMDL(NewCalibrator()),
		sequitur: NewSequitur(),
	}
}

/*
Chord is the tick's region set as one symbol, e.g. [R01-R02-R03]. The first
path element is the region; keys are sorted and deduplicated so the same set
on any tick is the same symbol.
*/
func Chord(tokens Tokens) string {
	seen := make(map[string]bool, len(tokens))
	keys := make([]string, 0, len(tokens))

	for _, token := range tokens {
		if len(token.Path) == 0 || token.Path[0] == "" || seen[token.Path[0]] {
			continue
		}

		seen[token.Path[0]] = true
		keys = append(keys, token.Path[0])
	}

	sort.Strings(keys)

	return "[" + strings.Join(keys, "-") + "]"
}

/*
Analyze feeds one tick's tokens as a single chord symbol through both
detectors and returns MDL's boundary verdict for this tick. LastChordRule
exposes the production Sequitur induced on this tick so callers can keep
addressing the region paths the grammar covers.
*/
func (sequencer *Sequencer) Analyze(tokens Tokens) (bool, int, []int, SegmentMeta) {
	chord := Chord(tokens)

	rule, _ := sequencer.sequitur.Analyze([]string{chord})
	sequencer.lastRule = rule

	var (
		boundary bool
		k        int
		events   []int
		meta     SegmentMeta
	)

	for _, b := range []byte(chord) {
		ok, at, found, seen := sequencer.mdl.Analyze(0, b)

		k, events, meta = at, found, seen

		if ok {
			boundary = true
		}
	}

	return boundary, k, events, meta
}

/*
LastChordRule is the production Sequitur induced on the latest tick, or nil.
TerminalNames on the grammar expands it to every region key the rule covers
across ticks.
*/
func (sequencer *Sequencer) LastChordRule() *Rule {
	return sequencer.lastRule
}

/*
Grammar exposes the underlying chord grammar for matching and persistence.
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
AnalyzeTokens is the Tokens-native entry the strategy layer calls: the tick's
region set becomes one chord symbol, Sequitur grows the grammar, and MDL
decides where the cut falls across the chord stream.
*/
func (sequencer *Sequencer) AnalyzeTokens(tokens Tokens) (*Rule, bool, SegmentMeta) {
	boundary, _, _, meta := sequencer.Analyze(tokens)

	return sequencer.lastRule, boundary, meta
}
