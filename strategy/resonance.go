package strategy

import (
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
)

const ignitionRegimeKey uint8 = 7

/*
Resonance reads coder layers on the tick, gates entry, and tracks in-trade
baselines for cliff exit while a lot is open.
*/
type Resonance struct {
	cliffs sync.Map
}

func NewResonance() *Resonance {
	return &Resonance{
		cliffs: sync.Map{},
	}
}

type Reading struct {
	SupportedHorizon float64
	Calibrated       float64
	Confidence       float64
	Surprise         float64
	Forward          []float64
	Skill1           float64
	Skill3           float64
	Energy           float64
	Layer0Err        float64
}

type cliffSample struct {
	peakHorizon int
	emaEnergy   float64
	emaLayer0   float64
	seeded      bool
}

func (r *Resonance) LayerKey(symbol, metric string) string {
	return strings.Join([]string{"resonance", symbolScope(symbol), metric}, "/")
}

func (r *Resonance) scalar(tick *data.Value, symbol, metric string) float64 {
	if tick == nil || symbol == "" {
		return 0
	}

	var values []float64
	tick.Lookup(&values, r.LayerKey(symbol, metric))

	if len(values) == 0 {
		return 0
	}

	return values[0]
}

func (r *Resonance) Read(tick *data.Value, symbol string) Reading {
	var out Reading

	if tick == nil || symbol == "" {
		return out
	}

	out.SupportedHorizon = r.scalar(tick, symbol, "supported_horizon")
	out.Calibrated = r.scalar(tick, symbol, "calibrated")
	out.Confidence = r.scalar(tick, symbol, "confidence")
	out.Surprise = r.scalar(tick, symbol, "surprise")
	out.Skill1 = r.scalar(tick, symbol, "task_skill_1")
	out.Skill3 = r.scalar(tick, symbol, "task_skill_3")
	out.Energy = r.scalar(tick, symbol, "energy")
	out.Layer0Err = r.scalar(tick, symbol, "layer_0_error")

	for horizon := 0; horizon < 16; horizon++ {
		value := r.scalar(tick, symbol, "forward_curve_"+strconv.Itoa(horizon))

		if horizon > 0 && value == 0 && len(out.Forward) == 0 {
			break
		}

		if horizon == 0 && value == 0 && r.scalar(tick, symbol, "forward_curve_1") == 0 {
			break
		}

		out.Forward = append(out.Forward, value)

		if int(out.SupportedHorizon) > 0 && horizon+1 >= int(out.SupportedHorizon) {
			break
		}
	}

	return out
}

func minSupportedHorizon() int {
	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.MinSupportedHorizon > 0 {
		return system.Cfg.Learning.MinSupportedHorizon
	}

	return 2
}

func (r *Resonance) FrictionAllows(regions []*nmstore.Region) (pass bool, checked bool) {
	if system.Cfg == nil || system.Cfg.Learning == nil {
		return true, false
	}

	cfg := system.Cfg.Learning

	if !cfg.RequireFrictionGate {
		return true, false
	}

	min := cfg.FrictionMinBrightness
	if min <= 0 {
		min = 0.35
	}

	for _, region := range regions {
		if region == nil {
			continue
		}

		if region.Key == 12 && region.Brightness >= min {
			return true, true
		}
	}

	return false, true
}

func ignitionLit(regions []*nmstore.Region) bool {
	if system.Cfg == nil || system.Cfg.Learning == nil {
		return true
	}

	cfg := system.Cfg.Learning

	if !cfg.RequireIgnitionRegime {
		return true
	}

	min := cfg.IgnitionMinBrightness
	if min <= 0 {
		min = 0.35
	}

	for _, region := range regions {
		if region == nil {
			continue
		}

		if region.Key == ignitionRegimeKey && region.Brightness >= min {
			return true
		}
	}

	return false
}

func (r *Resonance) ForwardAligned(reading Reading) bool {
	if len(reading.Forward) == 0 {
		return false
	}

	need := minSupportedHorizon()
	if need > len(reading.Forward) {
		need = len(reading.Forward)
	}

	var sign float64

	for i := 0; i < need; i++ {
		value := reading.Forward[i]

		if value == 0 {
			return false
		}

		here := math.Copysign(1, value)

		if sign == 0 {
			sign = here
			continue
		}

		if here != sign {
			return false
		}
	}

	return sign > 0
}

func (r *Resonance) MeanForward(reading Reading) float64 {
	if len(reading.Forward) == 0 {
		return 0
	}

	need := minSupportedHorizon()
	if need > len(reading.Forward) {
		need = len(reading.Forward)
	}

	var sum float64

	for i := 0; i < need; i++ {
		sum += reading.Forward[i]
	}

	return sum / float64(need)
}

func (r *Resonance) AllowsEntry(
	tick *data.Value,
	symbol string,
	regions []*nmstore.Region,
) bool {
	if tick == nil {
		return true
	}

	if !ignitionLit(regions) {
		return false
	}

	reading := r.Read(tick, symbol)

	if reading.Calibrated < 1 {
		return false
	}

	if int(reading.SupportedHorizon) < minSupportedHorizon() {
		return false
	}

	if system.Cfg != nil && system.Cfg.Resonance != nil && system.Cfg.Resonance.Gate {
		if reading.Confidence > 0 && reading.Confidence < 1 {
			return false
		}
	}

	return r.ForwardAligned(reading)
}

func (r *Resonance) cliff(symbol string) *cliffSample {
	if r == nil || symbol == "" {
		return &cliffSample{}
	}

	loaded, _ := r.cliffs.LoadOrStore(symbol, &cliffSample{})
	return loaded.(*cliffSample)
}

func (r *Resonance) ForgetHold(symbol string) {
	if r == nil || symbol == "" {
		return
	}

	r.cliffs.Delete(symbol)
}

func (r *Resonance) NoteHold(symbol string, tick *data.Value) {
	if r == nil || tick == nil || symbol == "" {
		return
	}

	reading := r.Read(tick, symbol)
	track := r.cliff(symbol)

	current := int(reading.SupportedHorizon)

	if current > track.peakHorizon {
		track.peakHorizon = current
	}

	const alpha = 0.12

	if !track.seeded {
		track.emaEnergy = reading.Energy
		track.emaLayer0 = reading.Layer0Err
		track.seeded = true
		return
	}

	track.emaEnergy = (1-alpha)*track.emaEnergy + alpha*reading.Energy
	track.emaLayer0 = (1-alpha)*track.emaLayer0 + alpha*reading.Layer0Err
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

func horizonCollapsed(track *cliffSample, supported int) bool {
	if track == nil || track.peakHorizon < 2 {
		return supported < 1
	}

	drop := cliffHorizonDrop()

	if track.peakHorizon >= 3 && supported <= 1 {
		return true
	}

	return supported <= track.peakHorizon-drop
}

func reconstructionSpike(track *cliffSample, reading Reading) bool {
	if track == nil || !track.seeded {
		return reading.Surprise > 0 && (reading.Energy > 0 || reading.Layer0Err > 0)
	}

	mult := cliffEnergyMult()

	if track.emaEnergy > 0 && reading.Energy > track.emaEnergy*mult {
		return true
	}

	if track.emaLayer0 > 0 && reading.Layer0Err > track.emaLayer0*mult {
		return true
	}

	if reading.Surprise > 0 && math.IsInf(reading.Surprise, 0) {
		return false
	}

	return reading.Surprise > track.emaEnergy*mult && reading.Surprise > 0
}

func (r *Resonance) CliffExit(
	tick *data.Value,
	symbol string,
	position *broker.Position,
) bool {
	if r == nil || tick == nil || position == nil || !position.Winning() {
		return false
	}

	reading := r.Read(tick, symbol)

	if reading.Calibrated < 1 {
		return false
	}

	track := r.cliff(symbol)
	supported := int(reading.SupportedHorizon)

	if !horizonCollapsed(track, supported) {
		return false
	}

	if !skillCollapsed(reading.Skill1) {
		return false
	}

	if track.peakHorizon >= 3 && reading.Skill3 > 0 && !skillCollapsed(reading.Skill3) {
		return false
	}

	return reconstructionSpike(track, reading)
}

func (r *Resonance) TryCliffExit(
	model *Model,
	out *data.Value,
	tick *data.Value,
	symbol string,
	pathKey string,
	virtual bool,
) bool {
	if r == nil || model == nil || model.desk == nil {
		return false
	}

	position := model.desk.Position(symbol, virtual)

	if position == nil || !r.CliffExit(tick, symbol, position) {
		return false
	}

	exitPath := model.emergencyExitPath(symbol, pathKey, position)

	if err := model.desk.Exit(symbol, exitPath, virtual); err != nil {
		return false
	}

	r.ForgetHold(symbol)
	position.AppendOutcomeTrigger("resonance cliff")

	if virtual {
		if model.finishVirtualExplore(position, out) && model.stage != nil {
			model.stage.ResetSequencer(symbol)
		}
	} else {
		model.path.Reset(symbol)
		if model.stage != nil {
			model.stage.ResetSequencer(symbol)
		}
	}

	if out != nil {
		out.Append(exitPath, "policyPath")
		out.Append("resonance cliff", "outcome")
		if virtual {
			out.Append("exit submitted", "outcome")
		}
	}

	return true
}
