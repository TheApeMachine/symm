package tokenizer

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
)

/*
Sequencer turns one tick's region brightness into a single path letter.
The letter is regime.posture: the brightest role in each band. Brightness
elects the role and stays out of the spelling. Risk (R12–R14) is a veto,
not a letter.

Each side (enter vs hold) keeps its own Sequitur sentence. Spell is the
tail of that sentence after digram compression, which is what Path follows
so oscillating chords do not inflate the trie one edge per tick.
*/
type Sequencer struct {
	enter       *Sequitur
	hold        *Sequitur
	active      *Sequitur
	mdl         *MDL
	lastEmitted string
	holding     bool
	enterBreak  bool
	holdBreak   bool
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
		mdl:   NewMDL(NewCalibrator()),
	}
}

// Reset clears the open sentences after a lot closes.
func (sequencer *Sequencer) Reset() {
	if sequencer == nil {
		return
	}
	sequencer.lastEmitted = ""
	sequencer.holding = false
	sequencer.enterBreak = false
	sequencer.holdBreak = false
	sequencer.enter.Sentence()
	sequencer.hold.Sentence()
}

var (
	// Regimes 6..11:
	// 6: Compression, 7: Ignition, 8: Friction, 9: Starved/Dead,
	// 10: Macro Recurrence, 11: Sentiment Recurrence
	flatRegime  = []uint8{6, 7, 8, 9, 10, 11}
	flatPosture = []uint8{0, 1, 2} // Enter postures: Spark, Support, Opposition

	holdRegime  = []uint8{6, 7, 8, 9, 10, 11}
	holdPosture = []uint8{3, 4, 5} // Exit postures: Stall, Evaporation, Reload

	riskRegions = []uint8{12, 13, 14} // Veto: Friction Hurdle, Contagion, Capacity
)

/*
Step emits one regime.posture token. Holding selects the exit bands;
otherwise the enter bands. The token is zero when either band is dark,
risk brightness beats the posture, or the label repeats the previous emit.
*/
func (sequencer *Sequencer) Step(lit []*store.Region, holding bool) StepResult {
	if len(lit) == 0 {
		return StepResult{Silence: StepSilenceEmpty}
	}

	regimeKeys, postureKeys := flatRegime, flatPosture
	if holding {
		regimeKeys, postureKeys = holdRegime, holdPosture
	}

	_, regime, regimeOK := brightest(lit, regimeKeys)
	postureBrightness, posture, postureOK := brightest(lit, postureKeys)

	if !regimeOK || !postureOK {
		return StepResult{Silence: StepSilenceEmpty, Peak: postureBrightness}
	}

	if risk, _, riskOK := brightest(lit, riskRegions); riskOK && risk > postureBrightness {
		return StepResult{Silence: StepSilenceVeto, Peak: risk}
	}

	label := fmt.Sprintf("%s.%s", RegionLabel(regime), RegionLabel(posture))

	if label == sequencer.lastEmitted {
		return StepResult{Silence: StepSilenceDuplicate, Peak: postureBrightness}
	}

	active := sequencer.sentence(holding)
	active.Analyze([]string{label})

	boundary, _, _, meta := sequencer.mdl.Analyze(sequencer.pos, labelCode(label))
	sequencer.pos++

	limit := 8
	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.MaxPathLength > 0 {
		limit = system.Cfg.Learning.MaxPathLength
	}
	spell := active.Tail(limit)

	if boundary {
		if holding {
			sequencer.holdBreak = true
		} else {
			sequencer.enterBreak = true
		}
	}

	sequencer.lastEmitted = label
	sequencer.holding = holding
	sequencer.active = active

	return StepResult{
		Token: NewToken([]string{label}, postureBrightness, meta),
		Spell: spell,
		Peak:  postureBrightness,
	}
}

func (sequencer *Sequencer) sentence(holding bool) *Sequitur {
	active := sequencer.enter
	pending := &sequencer.enterBreak
	if holding {
		active = sequencer.hold
		pending = &sequencer.holdBreak
		if !sequencer.holding {
			active.Sentence()
			*pending = false
		}
	} else if sequencer.holding {
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
	Holding     bool       `json:"holding"`
	EnterBreak  bool       `json:"enter_break"`
	HoldBreak   bool       `json:"hold_break"`
	Pos         uint32     `json:"pos"`
	EnterRoot   []string   `json:"enter_root"`
	HoldRoot    []string   `json:"hold_root"`
	EnterRules  []RuleView `json:"enter_rules"`
	HoldRules   []RuleView `json:"hold_rules"`
	MDL         MDLState   `json:"mdl"`
}

func (sequencer *Sequencer) State() SymbolState {
	if sequencer == nil {
		return SymbolState{}
	}

	return SymbolState{
		LastEmitted: sequencer.lastEmitted,
		Holding:     sequencer.holding,
		EnterBreak:  sequencer.enterBreak,
		HoldBreak:   sequencer.holdBreak,
		Pos:         sequencer.pos,
		EnterRoot:   sequencer.enter.RootSequence(),
		HoldRoot:    sequencer.hold.RootSequence(),
		EnterRules:  sequencer.enter.RuleViews(),
		HoldRules:   sequencer.hold.RuleViews(),
		MDL:         sequencer.mdl.State(),
	}
}

func (sequencer *Sequencer) Restore(state SymbolState) {
	if sequencer == nil {
		return
	}

	restoreGrammar(sequencer.enter, state.EnterRoot, state.EnterRules)
	restoreGrammar(sequencer.hold, state.HoldRoot, state.HoldRules)

	if sequencer.mdl == nil {
		sequencer.mdl = NewMDL(NewCalibrator())
	}
	sequencer.mdl.Restore(state.MDL)

	sequencer.lastEmitted = state.LastEmitted
	sequencer.holding = state.Holding
	sequencer.enterBreak = state.EnterBreak
	sequencer.holdBreak = state.HoldBreak
	sequencer.pos = state.Pos
	sequencer.active = sequencer.enter
	if state.Holding {
		sequencer.active = sequencer.hold
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
