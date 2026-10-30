package tokenizer

import (
	"fmt"

	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/system"
)

/*
Phase selects which story band spells this tick: enter (R00–R02), hold
(R06–R08), or exit (R03–R05). Each phase keeps its own Sequitur sentence.
*/
type Phase uint8

const (
	PhaseEnter Phase = iota
	PhaseHold
	PhaseExit
)

/*
Sequencer turns one tick's region brightness into a single path letter.
The letter is regime.posture: the brightest role in each band. Brightness
elects the role and stays out of the spelling. Risk (R12–R14) is a veto,
not a letter.

Spell is the tail of the active sentence after digram compression, which is
what the path trie follows so oscillating chords do not inflate one edge per tick.
*/
type Sequencer struct {
	enter       *Sequitur
	hold        *Sequitur
	exit        *Sequitur
	active      *Sequitur
	mdl         *MDL
	lastEmitted string
	lastPhase   Phase
	enterBreak  bool
	holdBreak   bool
	exitBreak   bool
	pos         uint32
}

// StepSilence explains why no path token was emitted this tick.
type StepSilence string

const (
	StepOK                StepSilence = ""
	StepSilenceEmpty      StepSilence = "empty"
	StepSilenceBrightness StepSilence = "brightness"
	StepSilenceDuplicate  StepSilence = "duplicate"
	StepSilenceVeto       StepSilence = "veto"
)

// StepResult is one tick's tokenizer outcome for path and telemetry.
// Spell is the current sentence in grammar form (terminals and rule refs).
type StepResult struct {
	Token   Token
	Spell   []string
	Silence StepSilence
	Peak    float64
}

func NewSequencer() *Sequencer {
	return &Sequencer{
		enter: NewSequitur(),
		hold:  NewSequitur(),
		exit:  NewSequitur(),
		mdl:   NewMDL(NewCalibrator()),
	}
}

// Reset clears the open sentences after a lot closes.
func (sequencer *Sequencer) Reset() {
	if sequencer == nil {
		return
	}
	sequencer.lastEmitted = ""
	sequencer.lastPhase = PhaseEnter
	sequencer.enterBreak = false
	sequencer.holdBreak = false
	sequencer.exitBreak = false
	sequencer.enter.Sentence()
	sequencer.hold.Sentence()
	sequencer.exit.Sentence()
}

var (
	// Regimes 6..11:
	// 6: Compression, 7: Ignition, 8: Friction, 9: Starved/Dead,
	// 10: Macro Recurrence, 11: Sentiment Recurrence
	flatRegime  = []uint8{6, 7, 8, 9, 10, 11}
	flatPosture = []uint8{0, 1, 2} // Enter postures: Spark, Support, Opposition

	holdRegime = []uint8{6, 7, 8, 9, 10, 11}

	riskRegions = []uint8{12, 13, 14} // Veto: Friction Hurdle, Contagion, Capacity
)

/*
Step emits one regime.posture token for the given lifecycle phase. The token
is empty when a band is dark, risk beats posture, or the label repeats.
*/
func (sequencer *Sequencer) Step(lit []*store.Region, phase Phase) StepResult {
	if len(lit) == 0 {
		return StepResult{Silence: StepSilenceEmpty}
	}

	regimeKeys := flatRegime
	switch phase {
	case PhaseHold, PhaseExit:
		regimeKeys = holdRegime
	}

	_, regime, regimeOK := brightest(lit, regimeKeys)

	var detKey, supKey, oppKey uint8

	switch phase {
	case PhaseHold:
		// HOLD: R06 detector, R07 support, R08 opposition (SIGNALS.md)
		detKey, supKey, oppKey = 6, 7, 8
	case PhaseExit:
		detKey, supKey, oppKey = 3, 4, 5
	default:
		detKey, supKey, oppKey = 0, 1, 2
	}

	var detector, support, opposition float64

	for _, r := range lit {
		if r == nil {
			continue
		}
		switch r.Key {
		case detKey:
			detector = r.Brightness
		case supKey:
			support = r.Brightness
		case oppKey:
			opposition = r.Brightness
		}
	}

	// ------------------------------------------------------------------------
	// NET FORCE VECTOR: (Detector + Support) - Opposition
	// This physical impulse is passed directly to the Tree to fight Resistance.
	// ------------------------------------------------------------------------
	netBrightness := (detector + support) - opposition

	// The token label still needs the dominant region to describe the state to the grammar
	posture := detKey
	maxVal := detector
	if support > maxVal {
		maxVal = support
		posture = supKey
	}
	if opposition > maxVal {
		maxVal = opposition
		posture = oppKey
	}

	if !regimeOK || maxVal <= 0 {
		return StepResult{Silence: StepSilenceEmpty, Peak: netBrightness}
	}

	// Risk Oracle (R12, R13, R14) vetoes only when net spark is bullish but
	// risk dominates that positive force. Opposition-led tokens carry negative
	// netBrightness; comparing risk to that would veto every bearish letter.
	if risk, _, riskOK := brightest(lit, riskRegions); riskOK && netBrightness > 0 && risk > netBrightness {
		return StepResult{Silence: StepSilenceVeto, Peak: risk}
	}

	label := fmt.Sprintf("%s.%s", RegionLabel(regime), RegionLabel(posture))

	if phase != sequencer.lastPhase {
		sequencer.lastEmitted = ""
	}

	if label == sequencer.lastEmitted {
		active := sequencer.sentence(phase)
		limit := 8
		if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.MaxPathLength > 0 {
			limit = system.Cfg.Learning.MaxPathLength
		}
		return StepResult{
			Silence: StepSilenceDuplicate,
			Token:   NewToken([]string{label}, netBrightness, 0),
			Spell:   active.Tail(limit),
			Peak:    netBrightness,
		}
	}

	active := sequencer.sentence(phase)
	active.Analyze([]string{label})

	boundary, _, _, meta := sequencer.mdl.Analyze(sequencer.pos, labelCode(label))
	sequencer.pos++

	limit := 8
	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.MaxPathLength > 0 {
		limit = system.Cfg.Learning.MaxPathLength
	}
	spell := active.Tail(limit)

	if boundary {
		switch phase {
		case PhaseHold:
			sequencer.holdBreak = true
		case PhaseExit:
			sequencer.exitBreak = true
		default:
			sequencer.enterBreak = true
		}
	}

	sequencer.lastEmitted = label
	sequencer.lastPhase = phase
	sequencer.active = active

	return StepResult{
		Token: NewToken([]string{label}, netBrightness, meta),
		Spell: spell,
		Peak:  netBrightness,
	}
}

func (sequencer *Sequencer) sentence(phase Phase) *Sequitur {
	active := sequencer.enter
	pending := &sequencer.enterBreak

	switch phase {
	case PhaseHold:
		active = sequencer.hold
		pending = &sequencer.holdBreak
	case PhaseExit:
		active = sequencer.exit
		pending = &sequencer.exitBreak
	}

	if phase != sequencer.lastPhase {
		active.Sentence()
		*pending = false
	}

	if *pending {
		active.Sentence()
		*pending = false
	}
	return active
}

func labelCode(label string) byte {
	var regime, posture int
	if _, err := fmt.Sscanf(label, "R%d.R%d", &regime, &posture); err != nil {
		return 0
	}
	return byte((regime << 4) | posture)
}

/*
brightest returns the highest strictly positive brightness among keys.
Equal brightness keeps the lower region key.
*/
func brightest(lit []*store.Region, keys []uint8) (float64, uint8, bool) {
	allowed := make(map[uint8]struct{}, len(keys))
	for _, key := range keys {
		allowed[key] = struct{}{}
	}

	var best float64
	var bestKey uint8
	found := false

	for _, region := range lit {
		if region == nil || region.Brightness <= 0 {
			continue
		}

		if _, ok := allowed[region.Key]; !ok {
			continue
		}

		if !found || region.Brightness > best || (region.Brightness == best && region.Key < bestKey) {
			best = region.Brightness
			bestKey = region.Key
			found = true
		}
	}

	return best, bestKey, found
}

func (sequencer *Sequencer) Grammar() *Sequitur {
	if sequencer == nil {
		return nil
	}
	if sequencer.active != nil {
		return sequencer.active
	}
	return sequencer.enter
}

func (sequencer *Sequencer) MDL() *MDL {
	return sequencer.mdl
}

// SymbolState is one symbol's sentences and MDL segmenter, saved beside explore paths.
type SymbolState struct {
	LastEmitted string     `json:"last_emitted"`
	LastPhase   Phase      `json:"last_phase"`
	EnterBreak  bool       `json:"enter_break"`
	HoldBreak   bool       `json:"hold_break"`
	ExitBreak   bool       `json:"exit_break"`
	Pos         uint32     `json:"pos"`
	EnterRoot   []string   `json:"enter_root"`
	HoldRoot    []string   `json:"hold_root"`
	ExitRoot    []string   `json:"exit_root"`
	EnterRules  []RuleView `json:"enter_rules"`
	HoldRules   []RuleView `json:"hold_rules"`
	ExitRules   []RuleView `json:"exit_rules"`
	MDL         MDLState   `json:"mdl"`
	// Holding is kept for backward-compatible grammar restores.
	Holding bool `json:"holding,omitempty"`
}

func (sequencer *Sequencer) State() SymbolState {
	if sequencer == nil {
		return SymbolState{}
	}

	return SymbolState{
		LastEmitted: sequencer.lastEmitted,
		LastPhase:   sequencer.lastPhase,
		EnterBreak:  sequencer.enterBreak,
		HoldBreak:   sequencer.holdBreak,
		ExitBreak:   sequencer.exitBreak,
		Pos:         sequencer.pos,
		EnterRoot:   sequencer.enter.RootSequence(),
		HoldRoot:    sequencer.hold.RootSequence(),
		ExitRoot:    sequencer.exit.RootSequence(),
		EnterRules:  sequencer.enter.RuleViews(),
		HoldRules:   sequencer.hold.RuleViews(),
		ExitRules:   sequencer.exit.RuleViews(),
		MDL:         sequencer.mdl.State(),
	}
}

func (sequencer *Sequencer) Restore(state SymbolState) {
	if sequencer == nil {
		return
	}

	restoreGrammar(sequencer.enter, state.EnterRoot, state.EnterRules)
	restoreGrammar(sequencer.hold, state.HoldRoot, state.HoldRules)
	restoreGrammar(sequencer.exit, state.ExitRoot, state.ExitRules)

	if sequencer.mdl == nil {
		sequencer.mdl = NewMDL(NewCalibrator())
	}
	sequencer.mdl.Restore(state.MDL)

	sequencer.lastEmitted = state.LastEmitted
	sequencer.lastPhase = state.LastPhase
	if sequencer.lastPhase == 0 && state.Holding {
		sequencer.lastPhase = PhaseHold
	}
	sequencer.enterBreak = state.EnterBreak
	sequencer.holdBreak = state.HoldBreak
	sequencer.exitBreak = state.ExitBreak
	sequencer.pos = state.Pos
	sequencer.active = sequencer.enter
	switch sequencer.lastPhase {
	case PhaseHold:
		sequencer.active = sequencer.hold
	case PhaseExit:
		sequencer.active = sequencer.exit
	}
}

func restoreGrammar(grammar *Sequitur, root []string, rules []RuleView) {
	if grammar == nil {
		return
	}
	for _, view := range rules {
		grammar.Learn(view.ID, view.Count, view.Symbols)
	}
	grammar.RestoreRoot(root)
}
