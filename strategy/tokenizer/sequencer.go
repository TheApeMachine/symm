package tokenizer

import (
	"strings"

	"github.com/theapemachine/symm/system"
)

/*
Sequencer wires Sequitur (grammar) and MDL (segment cuts). Step returns the
live cut: a Tokens sequence over the current grammar root tail.
*/
type Sequencer struct {
	grammar       *Sequitur
	mdl           *MDL
	lastEmitted   string
	sentenceBreak bool
}

func NewSequencer() *Sequencer {
	return &Sequencer{
		grammar: NewSequitur(),
		mdl:     NewMDL(),
	}
}

func (sequencer *Sequencer) Reset() {
	sequencer.lastEmitted = ""
	sequencer.sentenceBreak = false
	sequencer.grammar.Sentence()
}

/*
Step appends one path symbol into Sequitur, feeds the new tail symbol to MDL,
and returns the current cut as Tokens.
*/
func (sequencer *Sequencer) Step(symbol string, brightness float64) Tokens {
	return sequencer.StepPath([]string{symbol}, brightness)
}

/*
StepPath appends symbols into the grammar in order. Training feeds one regime
per call via Step; a repeated symbol only refreshes brightness.
*/
func (sequencer *Sequencer) StepPath(symbols []string, brightness float64) Tokens {
	parts := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		parts = append(parts, symbol)
	}

	if len(parts) == 0 {
		return NewTokens()
	}

	if sequencer.sentenceBreak {
		sequencer.grammar.Sentence()
		sequencer.sentenceBreak = false
	}

	key := strings.Join(parts, ".")

	if key == sequencer.lastEmitted {
		return sequencer.cut(brightness)
	}

	for _, symbol := range parts {
		sequencer.grammar.Analyze([]string{symbol})

		if tail := sequencer.grammar.Tail(1); len(tail) > 0 {
			if boundary, _ := sequencer.mdl.Analyze(tail[0]); boundary {
				sequencer.sentenceBreak = true
			}
		}
	}

	sequencer.lastEmitted = key

	return sequencer.cut(brightness)
}

func (sequencer *Sequencer) cut(tailBrightness float64) Tokens {
	segments := sequencer.grammar.Tail(system.Cfg.Learning.MaxPathLength)
	tokens := NewTokens()

	for i, segment := range segments {
		b := 0.0
		if i == len(segments)-1 {
			b = tailBrightness
		}

		tokens = tokens.Append(NewToken([]string{segment}, b))
	}

	return tokens
}

func (sequencer *Sequencer) Grammar() *Sequitur {
	return sequencer.grammar
}

// SequencerState is one grammar + MDL stream (persisted).
type SequencerState struct {
	LastEmitted string     `json:"last_emitted"`
	Root        []string   `json:"root"`
	Rules       []RuleView `json:"rules"`
	MDL         MDLState   `json:"mdl"`
}

func (sequencer *Sequencer) State() SequencerState {
	return SequencerState{
		LastEmitted: sequencer.lastEmitted,
		Root:        sequencer.grammar.RootSequence(),
		Rules:       sequencer.grammar.RuleViews(),
		MDL:         sequencer.mdl.State(),
	}
}

func (sequencer *Sequencer) Restore(state SequencerState) {
	restoreGrammar(sequencer.grammar, state.Root, state.Rules)
	sequencer.mdl.Restore(state.MDL)
	sequencer.lastEmitted = state.LastEmitted
	sequencer.sentenceBreak = false
}

func restoreGrammar(grammar *Sequitur, root []string, rules []RuleView) {
	for _, view := range rules {
		grammar.Learn(view.ID, view.Count, view.Symbols)
	}

	grammar.RestoreRoot(root)
}
