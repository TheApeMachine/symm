package logic

import (
	"math"
	"strings"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

type cognitiveBranch struct {
	ID          int     `json:"id"`
	ParentID    int     `json:"parentId"`
	Token       string  `json:"token"`
	Prefix      string  `json:"prefix"`
	Depth       int     `json:"depth"`
	Probability float64 `json:"probability"`
	Count       uint64  `json:"count"`
}

type cognitiveBeam struct {
	Sequence string  `json:"sequence"`
	Score    float64 `json:"score"`
}

type cognitiveClass struct {
	Name        string  `json:"name"`
	Probability float64 `json:"probability"`
}

/*
Publish sends the analyzer's current logic outputs to the UI websocket.
*/
func (analyzer *Analyzer) Publish(
	symbol string,
	measurements []*types.Measurement,
	thesis *strategy.Thesis,
) {
	if analyzer.uiHub == nil {
		return
	}

	at, ok := latestAt(measurements)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic analyzer: publication timestamp required",
			nil,
		))

		return
	}

	output := datura.Map[any]{}
	manifold := analyzer.manifolds[symbol]

	if manifoldFrame, ok := analyzer.manifold(symbol, at, thesis); ok {
		output["manifold"] = []any{manifoldFrame}
	}

	if resonanceFrame, ok := analyzer.resonance(symbol, at, thesis); ok {
		output["resonance"] = []any{resonanceFrame}
	}

	if causalFrame, ok := analyzer.causal(symbol, at, thesis); ok {
		output["causal"] = []any{causalFrame}
	}

	if cognitiveReading, ok := manifold.cognitive(symbol, at); ok {
		output["cognitive"] = datura.Map[any]{
			"readings": datura.Map[any]{
				symbol: cognitiveReading,
			},
		}
	}

	if len(output) == 0 {
		return
	}

	if analyzer.uiHub != nil && analyzer.uiHub.Messages != nil {
		select {
		case analyzer.uiHub.Messages <- output.Marshal():
		default:
		}
	}
}

func (analyzer *Analyzer) manifold(
	symbol string,
	at time.Time,
	thesis *strategy.Thesis,
) (map[string]any, bool) {
	manifold := analyzer.manifolds[symbol]

	if manifold == nil {
		return nil, false
	}

	snapshot, ok := thesis.Evidence("manifold")

	if !ok {
		return nil, false
	}

	reading, ok := snapshot.(pmanifold.Reading)

	if !ok || !reading.IsFinite() {
		return nil, false
	}

	rho, err := manifold.solver.ReadRhoProjection()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic analyzer: failed to read rho projection",
			err,
		))

		return nil, false
	}

	frame := map[string]any{
		"source": "manifold",
		"symbol": symbol,
		"at":     at.UTC().Format(time.RFC3339Nano),
		"grid": datura.Map[any]{
			"x": manifold.config.GridX,
			"y": manifold.config.GridY,
			"z": manifold.config.GridZ,
		},
		"reading":     reading,
		"rho":         rho,
		"peak":        peak(rho),
		"momentum":    reading.GuidanceSpeed,
		"oscillators": datura.Map[any]{"coherence": reading.CoherenceMag2},
	}

	if len(manifold.classes.Winner) > 0 {
		frame["category"] = string(manifold.classes.Winner)
	}

	particles := manifold.particles()

	if len(particles) > 0 {
		frame["particles"] = particles
	}

	return frame, true
}

func (analyzer *Analyzer) resonance(
	symbol string,
	at time.Time,
	thesis *strategy.Thesis,
) (map[string]any, bool) {
	snapshot, ok := thesis.Evidence("resonance")

	if !ok {
		return nil, false
	}

	outcome, ok := snapshot.(ResonanceOutcome)

	if !ok || !finiteSlice(outcome.Latent) ||
		!finite(outcome.Energy) ||
		!finite(outcome.Surprise) ||
		!finite(outcome.ReturnForecast) {
		return nil, false
	}

	return map[string]any{
		"source":   "resonance",
		"symbol":   symbol,
		"at":       at.UTC().Format(time.RFC3339Nano),
		"latent":   outcome.Latent,
		"energy":   outcome.Energy,
		"surprise": outcome.Surprise,
		"flow":     outcome.ReturnForecast,
		"baseline": 0,
	}, true
}

func (analyzer *Analyzer) causal(
	symbol string,
	at time.Time,
	thesis *strategy.Thesis,
) (map[string]any, bool) {
	snapshot, ok := thesis.Evidence("causal")

	if !ok {
		return nil, false
	}

	output, ok := snapshot.(algorithm.PearlOutput)

	if !ok {
		return nil, false
	}

	frame := output.Outputs()
	frame["source"] = "causal"
	frame["symbol"] = symbol
	frame["at"] = at.UTC().Format(time.RFC3339Nano)
	frame["beta"] = output.Association
	frame["baseline"] = output.EntryBaseline
	frame["panic"] = output.Noise

	return frame, true
}

func (manifold *Manifold) cognitive(symbol string, at time.Time) (map[string]any, bool) {
	if manifold == nil || len(manifold.sequence) == 0 {
		return nil, false
	}

	branches := manifold.branches()
	beams := manifold.beams()
	classes := manifold.classifications()
	entropyBits := manifold.entropy()
	contrast := contrastEvidence(classes)
	winner := ""

	if len(manifold.classes.Winner) > 0 {
		winner = string(manifold.classes.Winner)
	}

	lookaheadScore := 0.0

	if len(beams) > 0 {
		lookaheadScore = beams[0].Score
	}

	return map[string]any{
		"scope":            symbol,
		"sequence":         string(manifold.sequence),
		"regimePrefix":     winner,
		"regimeCohort":     len(classes),
		"ambiguous":        len(classes) == 0 || contrast <= 0,
		"sideline":         len(classes) == 0,
		"entropyBits":      entropyBits,
		"entropyThreshold": entropyThreshold(classes),
		"classConfidence":  manifold.classes.Highest,
		"contrastEvidence": contrast,
		"lookaheadScore":   lookaheadScore,
		"lookaheadPaths":   len(beams),
		"winnerClass":      winner,
		"updatedAt":        at.UnixMilli(),
		"beamWidth":        len(beams),
		"maxHops":          len(strings.Split(string(manifold.sequence), "_")),
		"nodeCount":        len(branches),
		"branches":         branches,
		"beams":            beams,
		"classes":          classes,
	}, true
}

func (manifold *Manifold) particles() []map[string]any {
	oscillators, err := manifold.solver.ReadOscillators(len(types.CategoryOrder))

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic analyzer: failed to read manifold oscillators",
			err,
		))

		return nil
	}

	particles := make([]map[string]any, 0, len(oscillators))

	for index, oscillator := range oscillators {
		if !finite(oscillator.Phase) ||
			!finite(oscillator.Omega) ||
			!finite(oscillator.Amplitude) ||
			!finite(oscillator.Heat) ||
			!finite(oscillator.PosX) ||
			!finite(oscillator.PosY) ||
			!finite(oscillator.PosZ) ||
			!finite(oscillator.VelX) ||
			!finite(oscillator.VelY) ||
			!finite(oscillator.VelZ) {
			continue
		}

		role := ""

		if index < len(types.CategoryOrder) {
			role = string(types.CategoryOrder[index])
		}

		particles = append(particles, map[string]any{
			"source":    "manifold",
			"role":      role,
			"cell_x":    oscillator.PosX,
			"cell_y":    oscillator.PosY,
			"cell_z":    oscillator.PosZ,
			"phase":     oscillator.Phase,
			"omega":     oscillator.Omega,
			"amplitude": oscillator.Amplitude,
			"heat":      oscillator.Heat,
			"vel_x":     oscillator.VelX,
			"vel_y":     oscillator.VelY,
			"vel_z":     oscillator.VelZ,
			"speed": math.Hypot(
				oscillator.VelX,
				math.Hypot(oscillator.VelY, oscillator.VelZ),
			),
		})
	}

	return particles
}

func (manifold *Manifold) branches() []cognitiveBranch {
	tokens := strings.Split(string(manifold.sequence), "_")
	branches := []cognitiveBranch{{
		ID:          0,
		ParentID:    -1,
		Token:       "root",
		Prefix:      "",
		Depth:       0,
		Probability: 1,
		Count:       manifold.tree.GetContextWeight(nil).Count,
	}}
	prefix := ""

	for index, token := range tokens {
		if token == "" {
			continue
		}

		if prefix == "" {
			prefix = token
		} else {
			prefix += "_" + token
		}

		weight := manifold.tree.GetContextWeight([]byte(prefix))
		probability := weight.Probability

		if probability == 0 && index < len(manifold.surprisals) {
			probability = math.Exp2(-manifold.surprisals[index].Surprisal)
		}

		branches = append(branches, cognitiveBranch{
			ID:          len(branches),
			ParentID:    len(branches) - 1,
			Token:       token,
			Prefix:      prefix,
			Depth:       index + 1,
			Probability: probability,
			Count:       weight.Count,
		})
	}

	return branches
}

func (manifold *Manifold) beams() []cognitiveBeam {
	beams := make([]cognitiveBeam, 0, len(manifold.lookahead))

	for _, lookahead := range manifold.lookahead {
		if len(lookahead.Token) == 0 || !finite(lookahead.Probability) ||
			lookahead.Probability <= 0 {
			continue
		}

		beams = append(beams, cognitiveBeam{
			Sequence: string(lookahead.Token),
			Score:    math.Log(lookahead.Probability),
		})
	}

	return beams
}

func (manifold *Manifold) classifications() []cognitiveClass {
	classes := make([]cognitiveClass, 0, len(manifold.classes.Scores))

	for _, score := range manifold.classes.Scores {
		if len(score.ClassName) == 0 || !finite(score.Value) {
			continue
		}

		classes = append(classes, cognitiveClass{
			Name:        string(score.ClassName),
			Probability: score.Value,
		})
	}

	return classes
}

func (manifold *Manifold) entropy() float64 {
	entropyBits := 0.0

	for _, surprisal := range manifold.surprisals {
		if finite(surprisal.Surprisal) {
			entropyBits += surprisal.Surprisal
		}
	}

	return entropyBits
}

func latestAt(measurements []*types.Measurement) (time.Time, bool) {
	var latest time.Time

	for _, measurement := range measurements {
		if measurement.At.IsZero() {
			continue
		}

		if latest.IsZero() || measurement.At.After(latest) {
			latest = measurement.At
		}
	}

	return latest, !latest.IsZero()
}

func peak(matrix [][]float64) float64 {
	value := 0.0

	for _, row := range matrix {
		for _, cell := range row {
			if finite(cell) && cell > value {
				value = cell
			}
		}
	}

	return value
}

func contrastEvidence(classes []cognitiveClass) float64 {
	if len(classes) == 0 {
		return 0
	}

	if len(classes) == 1 {
		return classes[0].Probability
	}

	return classes[0].Probability - classes[1].Probability
}

func entropyThreshold(classes []cognitiveClass) float64 {
	if len(classes) == 0 || classes[0].Probability <= 0 {
		return 0
	}

	return -math.Log2(classes[0].Probability)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finiteSlice(values []float64) bool {
	for _, value := range values {
		if !finite(value) {
			return false
		}
	}

	return true
}
