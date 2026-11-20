package tokenizer

import (
	"strings"
	"sync"

	"github.com/theapemachine/symm/system"
)

/*
Sequencer wires Sequitur (grammar) and MDL (segment cuts). Step returns the
live cut plus whether grammar/MDL state mutated.

mu serializes Training.Step (Ring 3) against Model resetPath → ResetSequencer
(Ring 4) so Tick N+1 cannot mutate grammar while Tick N still resets it.
*/
type Sequencer struct {
	mu            sync.Mutex
	grammar       *Sequitur
	mdl           *MDL
	lastEmitted   string
	sentenceBreak bool
}

/*
NewSequencer pairs a fresh Sequitur with MDL — Training owns one per symbol map key.
*/
func NewSequencer() *Sequencer {
	return &Sequencer{
		grammar: NewSequitur(),
		mdl:     NewMDL(),
	}
}

/*
Reset clears the live sentence and lastEmitted after model resetPath without
dropping learned rules — next Step starts a new phrase using existing M-rules.
*/
func (sequencer *Sequencer) Reset() {
	sequencer.mu.Lock()
	defer sequencer.mu.Unlock()
	sequencer.lastEmitted = ""
	sequencer.sentenceBreak = false
	sequencer.grammar.Sentence()
}

/*
Peek returns the live cut without appending — quiet grid ticks must not wipe
envelope Paths to Len=0 or MinPathLength seals every idle frame.
*/
func (sequencer *Sequencer) Peek(brightness float64) Tokens {
	sequencer.mu.Lock()
	defer sequencer.mu.Unlock()
	return sequencer.cut(brightness)
}

/*
Step appends one path symbol into Sequitur, feeds the new tail symbol to MDL,
and returns the current cut plus whether grammar/MDL state mutated.
*/
func (sequencer *Sequencer) Step(symbol string, brightness float64) (Tokens, bool) {
	return sequencer.StepPath([]string{symbol}, brightness)
}

/*
StepPath appends symbols into the grammar in order. Training feeds one regime
per call via Step; a repeated symbol only refreshes brightness (mutated=false).
*/
func (sequencer *Sequencer) StepPath(symbols []string, brightness float64) (Tokens, bool) {
	sequencer.mu.Lock()
	defer sequencer.mu.Unlock()

	parts := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		parts = append(parts, symbol)
	}

	if len(parts) == 0 {
		return NewTokens(), false
	}

	mutated := false

	if sequencer.sentenceBreak {
		sequencer.grammar.Sentence()
		sequencer.sentenceBreak = false
		mutated = true
	}

	key := strings.Join(parts, ".")

	if key == sequencer.lastEmitted {
		return sequencer.cut(brightness), mutated
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

	return sequencer.cut(brightness), true
}

/*
cut projects grammar Tail into runtime.Tokens with brightness only on the tail
segment — Model.Predict compares Last().Brightness against tree resistance.

M-rules are expanded to terminals so the global Tree never sees per-symbol
aliases (BTC's M1 ≠ DOGE's M1).
*/
func (sequencer *Sequencer) cut(tailBrightness float64) Tokens {
	segments := sequencer.grammar.Tail(system.Cfg.Learning.MaxPathLength)
	expanded := make([]string, 0, len(segments)*2)

	for _, segment := range segments {
		terms := sequencer.grammar.TerminalsForRef(segment)
		if len(terms) == 0 {
			expanded = append(expanded, segment)
			continue
		}
		expanded = append(expanded, terms...)
	}

	if max := system.Cfg.Learning.MaxPathLength; max > 0 && len(expanded) > max {
		expanded = expanded[len(expanded)-max:]
	}

	tokens := NewTokens()

	for i, segment := range expanded {
		b := 0.0
		if i == len(expanded)-1 {
			b = tailBrightness
		}

		tokens = tokens.Append(NewToken([]string{segment}, b))
	}

	return tokens
}

/*
SequencerState is everything needed to resume path grammar after restart — root,
rules, MDL segmenter, and lastEmitted dedupe guard.
*/
type SequencerState struct {
	LastEmitted string     `json:"last_emitted"`
	Root        []string   `json:"root"`
	Rules       []RuleView `json:"rules"`
	MDL         MDLState   `json:"mdl"`
}

/*
State exports grammar and MDL for GrammarCatalog — called synchronously on mutation
so queued store writes cannot reorder older snapshots ahead of newer cuts.
*/
func (sequencer *Sequencer) State() SequencerState {
	sequencer.mu.Lock()
	defer sequencer.mu.Unlock()
	return SequencerState{
		LastEmitted: sequencer.lastEmitted,
		Root:        sequencer.grammar.RootSequence(),
		Rules:       sequencer.grammar.RuleViews(),
		MDL:         sequencer.mdl.State(),
	}
}

/*
Restore replays Learn/RestoreRoot instead of rediscovering rules from scratch.
*/
func (sequencer *Sequencer) Restore(state SequencerState) {
	sequencer.mu.Lock()
	defer sequencer.mu.Unlock()
	restoreGrammar(sequencer.grammar, state.Root, state.Rules)
	sequencer.mdl.Restore(state.MDL)
	sequencer.lastEmitted = state.LastEmitted
	sequencer.sentenceBreak = false
}

/*
restoreGrammar loads productions before root so digram indexes exist when the
sentence is reinstalled — order matches Persist round-trip from Sequencer.State.
*/
func restoreGrammar(grammar *Sequitur, root []string, rules []RuleView) {
	for _, view := range rules {
		grammar.Learn(view.ID, view.Count, view.Symbols)
	}

	grammar.RestoreRoot(root)
}
