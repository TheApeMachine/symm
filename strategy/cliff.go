package strategy

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/system"
)

/*
cliffTrack holds per-symbol coder baselines while a lot is open so
emergency exit can detect horizon collapse and reconstruction spikes.
*/
type cliffTrack struct {
	peakHorizon int
	emaEnergy   float64
	emaLayer0   float64
	seeded      bool
}

type cliffReading struct {
	coderReading
	skill1    float64
	skill3    float64
	energy    float64
	layer0Err float64
}

func cliffReadingFromTick(tick *data.Value, symbol string) cliffReading {
	base := coderReadingFromTick(tick, symbol)

	return cliffReading{
		coderReading: base,
		skill1:       readScalarFromTick(tick, symbol, "task_skill_1"),
		skill3:       readScalarFromTick(tick, symbol, "task_skill_3"),
		energy:       readScalarFromTick(tick, symbol, "energy"),
		layer0Err:    readScalarFromTick(tick, symbol, "layer_0_error"),
	}
}

func (model *Model) cliffState(symbol string) *cliffTrack {
	if model == nil || symbol == "" {
		return &cliffTrack{}
	}

	if model.cliff == nil {
		model.cliff = &sync.Map{}
	}

	loaded, _ := model.cliff.LoadOrStore(symbol, &cliffTrack{})
	return loaded.(*cliffTrack)
}

func (model *Model) resetCliffTrack(symbol string) {
	if model == nil || model.cliff == nil || symbol == "" {
		return
	}

	model.cliff.Delete(symbol)
}

func (model *Model) updateCliffTrack(symbol string, reading cliffReading) {
	track := model.cliffState(symbol)

	current := int(reading.supportedHorizon)

	if current > track.peakHorizon {
		track.peakHorizon = current
	}

	const alpha = 0.12

	if !track.seeded {
		track.emaEnergy = reading.energy
		track.emaLayer0 = reading.layer0Err
		track.seeded = true
		return
	}

	track.emaEnergy = (1-alpha)*track.emaEnergy + alpha*reading.energy
	track.emaLayer0 = (1-alpha)*track.emaLayer0 + alpha*reading.layer0Err
}

func cliffHorizonDrop() int {
	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.CliffHorizonDrop > 0 {
		return system.Cfg.Learning.CliffHorizonDrop
	}

	return 2
}

func cliffEnergyMult() float64 {
	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.CliffEnergyMult > 0 {
		return system.Cfg.Learning.CliffEnergyMult
	}

	return 1.4
}

func skillCollapsed(skill float64) bool {
	return skill > 0 && skill < 1
}

func positionWinning(position *broker.Position) bool {
	if position == nil || position.PnL == nil {
		return false
	}

	return position.PnL.Sign() > 0
}

func horizonCollapsed(track *cliffTrack, supported int) bool {
	if track == nil || track.peakHorizon < 2 {
		return supported < 1
	}

	drop := cliffHorizonDrop()

	if track.peakHorizon >= 3 && supported <= 1 {
		return true
	}

	return supported <= track.peakHorizon-drop
}

func reconstructionSpike(track *cliffTrack, reading cliffReading) bool {
	if track == nil || !track.seeded {
		return reading.surprise > 0 && (reading.energy > 0 || reading.layer0Err > 0)
	}

	mult := cliffEnergyMult()

	if track.emaEnergy > 0 && reading.energy > track.emaEnergy*mult {
		return true
	}

	if track.emaLayer0 > 0 && reading.layer0Err > track.emaLayer0*mult {
		return true
	}

	if reading.surprise > 0 && math.IsInf(reading.surprise, 0) {
		return false
	}

	return reading.surprise > track.emaEnergy*mult && reading.surprise > 0
}

/*
resonanceCliffExit is the asymmetric cliff detector: winning lots (paper
or virtual) exit when multi-horizon skill collapses, supported reach falls
off its in-trade peak, and reconstruction energy or layer-0 error spikes.
*/
func (model *Model) resonanceCliffExit(
	tick *data.Value,
	symbol string,
	position *broker.Position,
) bool {
	if model == nil || tick == nil || position == nil {
		return false
	}

	if !positionWinning(position) {
		return false
	}

	reading := cliffReadingFromTick(tick, symbol)

	if reading.calibrated < 1 {
		return false
	}

	track := model.cliffState(symbol)
	supported := int(reading.supportedHorizon)

	if !horizonCollapsed(track, supported) {
		return false
	}

	if !skillCollapsed(reading.skill1) {
		return false
	}

	if track.peakHorizon >= 3 && reading.skill3 > 0 && !skillCollapsed(reading.skill3) {
		return false
	}

	return reconstructionSpike(track, reading)
}

func (model *Model) emergencyExitPath(
	symbol string,
	pathKey string,
	position *broker.Position,
) string {
	if position == nil {
		return pathKey
	}

	if pathDepth(pathKey) >= minPathDepth() && pathKey != position.EntryPath {
		return pathKey
	}

	if trigger := model.path.TriggerLive(symbol); pathDepth(trigger) >= minPathDepth() {
		return trigger
	}

	if live := model.path.Live(symbol); pathDepth(live) >= minPathDepth() && live != position.EntryPath {
		return live
	}

	if position.EntryPath != "" {
		return position.EntryPath + "/R03"
	}

	return pathKey
}

func (model *Model) tryResonanceCliffExit(
	out *data.Value,
	tick *data.Value,
	symbol string,
	pathKey string,
	virtual bool,
) bool {
	position := model.desk.Position(symbol, virtual)

	if position == nil || !model.resonanceCliffExit(tick, symbol, position) {
		return false
	}

	exitPath := model.emergencyExitPath(symbol, pathKey, position)

	if err := model.desk.Exit(symbol, exitPath, virtual); err != nil {
		return false
	}

	model.path.Reset(symbol)
	model.stage.ResetSequencer(symbol)
	model.resetCliffTrack(symbol)
	position.AppendOutcomeTrigger("resonance cliff")

	if out != nil {
		out.Append(exitPath, "policyPath")
		out.Append("resonance cliff", "outcome")
	}

	return true
}
