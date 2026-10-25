package tokenizer

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/store"
)

/*
Sequencer turns one tick's region brightness into a single path letter.
The letter is regime.posture: the brightest role in each band. Brightness
elects the role and stays out of the spelling. Risk (R12–R14) is a veto,
not a letter.
*/
type Sequencer struct {
	sequitur    *Sequitur
	mdl         *MDL
	lastEmitted string
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
type StepResult struct {
	Token   Token
	Silence StepSilence
	Peak    float64
}

func NewSequencer() *Sequencer {
	return &Sequencer{
		sequitur: NewSequitur(),
		mdl:      NewMDL(NewCalibrator()),
	}
}

// Reset clears the last letter after a lot closes, so the next entry can spell it again.
func (sequencer *Sequencer) Reset() {
	if sequencer == nil {
		return
	}
	sequencer.lastEmitted = ""
}

var (
	flatRegime  = []uint8{6, 7, 8}
	flatPosture = []uint8{0, 1, 2}
	holdRegime  = []uint8{7, 8, 9}
	holdPosture = []uint8{3, 4, 5}
	riskRegions = []uint8{12, 13, 14}
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

	sequencer.lastEmitted = label

	return StepResult{
		Token: NewToken([]string{label}, postureBrightness, 0),
		Peak:  postureBrightness,
	}
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
	return sequencer.sequitur
}

func (sequencer *Sequencer) MDL() *MDL {
	return sequencer.mdl
}
