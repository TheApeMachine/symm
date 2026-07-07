package trader

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/geometry"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

const mortonCoordinateMax = uint64(1<<32 - 1)

type Topology struct {
	morton  *datura.MortonCoder
	sources map[types.SourceType]uint64
	labels  map[string]string
}

type cortexSequence struct {
	Symbol  string
	Display []string
	Tree    []string
	Class   string
}

func newTopology() *Topology {
	sources := make(map[types.SourceType]uint64, len(cortexSourceOrder)+3)

	for index, source := range cortexSourceOrder {
		sources[source] = uint64(index + 1)
	}

	sources[types.SourceManifold] = uint64(len(sources) + 1)
	sources[types.SourceResonance] = uint64(len(sources) + 1)
	sources[types.SourceCausal] = uint64(len(sources) + 1)

	return &Topology{
		morton:  datura.NewMortonCoder(),
		sources: sources,
		labels:  map[string]string{},
	}
}

func (topology *Topology) Sequence(
	observation *cortexObservation,
) (cortexSequence, error) {
	if topology == nil {
		return cortexSequence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"cortex topology: topology is not initialized",
			nil,
		))
	}

	if observation == nil {
		return cortexSequence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"cortex topology: observation is required",
			nil,
		))
	}

	sequence := cortexSequence{
		Symbol: token(observation.symbol),
		Class:  observation.class(),
	}

	for _, source := range cortexSourceOrder {
		reading := observation.measurements[source]

		if reading.category.Type == types.CategoryTypeNone {
			continue
		}

		if err := topology.addCategory(
			&sequence,
			source,
			reading.category,
			reading.metrics,
		); err != nil {
			return cortexSequence{}, err
		}
	}

	if observation.manifold != nil {
		if err := topology.addManifold(&sequence, observation.manifold); err != nil {
			return cortexSequence{}, err
		}
	}

	if observation.resonance != nil {
		if err := topology.addResonance(&sequence, observation.resonance); err != nil {
			return cortexSequence{}, err
		}
	}

	if observation.causal != nil {
		if err := topology.addCausal(&sequence, observation.causal); err != nil {
			return cortexSequence{}, err
		}
	}

	return sequence, nil
}

func (topology *Topology) Label(treeToken string) string {
	if topology == nil {
		return treeToken
	}

	label := topology.labels[treeToken]

	if label == "" {
		return treeToken
	}

	return label
}

func (topology *Topology) DisplayPath(rootPrefix string, sequence string) string {
	trimmed := displaySequence(rootPrefix, sequence)

	if trimmed == "" {
		return ""
	}

	parts := strings.Split(trimmed, "_")

	for index, part := range parts {
		parts[index] = topology.Label(part)
	}

	return strings.Join(parts, "_")
}

func (topology *Topology) addCategory(
	sequence *cortexSequence,
	source types.SourceType,
	category types.Category,
	metrics map[string]float64,
) error {
	label := tokenPair(string(source), string(category.Type))
	features := append(
		[]float64{category.Confidence, category.Strength, category.Surprisal},
		continuousFeatures(metrics)...,
	)
	treeToken, err := topology.token(
		label,
		source,
		category.Type,
		features,
	)

	if err != nil {
		return err
	}

	sequence.Display = append(sequence.Display, label)
	sequence.Tree = append(sequence.Tree, treeToken)

	return nil
}

/*
continuousFeatures returns the signal's raw continuous state vector in a stable
key order so the same market state always encodes to the same token. Price is
excluded because it is per-tick bookkeeping, not part of the mechanical state.
This is what preserves magnitude (reynolds, vorticity, spectral radius, ...)
into the tree instead of collapsing it to the winning category label.
*/
func continuousFeatures(metrics map[string]float64) []float64 {
	if len(metrics) == 0 {
		return nil
	}

	keys := make([]string, 0, len(metrics))

	for key := range metrics {
		if key == "price" {
			continue
		}

		keys = append(keys, key)
	}

	sort.Strings(keys)

	features := make([]float64, 0, len(keys))

	for _, key := range keys {
		features = append(features, metrics[key])
	}

	return features
}

func (topology *Topology) addManifold(
	sequence *cortexSequence,
	frame *logic.ManifoldFrame,
) error {
	label := tokenPair(string(types.SourceManifold), string(frame.Category))
	treeToken, err := topology.token(
		label,
		types.SourceManifold,
		frame.Category,
		[]float64{
			frame.Strength,
			frame.Momentum,
			frame.Pressure,
			frame.Shock,
			frame.Resistance,
			frame.Peak,
			frame.Summary.CenterX,
			frame.Summary.CenterZ,
			frame.Summary.Gradient,
			frame.Reading.CoherenceMag2,
		},
	)

	if err != nil {
		return err
	}

	sequence.Display = append(sequence.Display, label)
	sequence.Tree = append(sequence.Tree, treeToken)

	return nil
}

func (topology *Topology) addResonance(
	sequence *cortexSequence,
	frame *logic.ResonanceFrame,
) error {
	label := tokenPair(string(types.SourceResonance), string(frame.Category))
	features := []float64{
		frame.Confidence,
		frame.Flow,
		frame.Stress,
		frame.Coupling,
		frame.Baseline,
		frame.Energy,
		frame.Surprise,
	}
	features = append(features, frame.Latent...)
	treeToken, err := topology.token(
		label,
		types.SourceResonance,
		frame.Category,
		features,
	)

	if err != nil {
		return err
	}

	sequence.Display = append(sequence.Display, label)
	sequence.Tree = append(sequence.Tree, treeToken)

	return nil
}

func (topology *Topology) addCausal(
	sequence *cortexSequence,
	frame *logic.CausalFrame,
) error {
	label := tokenPair(string(types.SourceCausal), string(frame.Category))
	treeToken, err := topology.token(
		label,
		types.SourceCausal,
		frame.Category,
		[]float64{
			frame.Confidence,
			frame.Strength,
			frame.Baseline,
			frame.Uplift,
			frame.Intervention,
			frame.Beta,
			frame.Panic,
			frame.Residual,
		},
	)

	if err != nil {
		return err
	}

	sequence.Display = append(sequence.Display, label)
	sequence.Tree = append(sequence.Tree, treeToken)

	return nil
}

func (topology *Topology) token(
	label string,
	source types.SourceType,
	category types.CategoryType,
	features []float64,
) (string, error) {
	value, evidence, err := topology.value(source, category, features)

	if err != nil {
		return "", err
	}

	x := quantizeUnit(phaseUnit(value))
	y := quantizeUnit(evidence)
	out := fmt.Sprintf("m%016x", topology.morton.Encode(x, y))
	topology.labels[out] = label

	return out, nil
}

func (topology *Topology) value(
	source types.SourceType,
	category types.CategoryType,
	features []float64,
) (geometry.Value, float64, error) {
	sourceIndex := topology.sources[source]

	if sourceIndex == 0 {
		return geometry.Value{}, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("cortex topology: unknown source %q", source),
			nil,
		))
	}

	categoryIndex := types.CategoryIndex(category)

	if categoryIndex == 0 {
		return geometry.Value{}, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("cortex topology: unknown category %q", category),
			nil,
		))
	}

	var value geometry.Value
	value[0] = sourceIndex
	value[1] = uint64(categoryIndex)

	evidence := 0.0
	evidenceCount := 0

	for index, feature := range features {
		if math.IsInf(feature, 0) || math.IsNaN(feature) {
			return geometry.Value{}, 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"cortex topology: non-finite feature",
				nil,
			))
		}

		unit := scalarUnit(feature)
		evidence += unit
		evidenceCount++

		if index+2 < len(value) {
			value[index+2] = quantizeUnit(unit)
		}
	}

	if evidenceCount == 0 {
		return geometry.Value{}, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"cortex topology: at least one feature is required",
			nil,
		))
	}

	return value, evidence / float64(evidenceCount), nil
}

func phaseUnit(value geometry.Value) float64 {
	dial := geometry.NewPhaseDial().EncodeFromValues([]geometry.Value{value})
	realSum := 0.0
	imaginarySum := 0.0

	for _, component := range dial {
		realSum += real(component)
		imaginarySum += imag(component)
	}

	angle := math.Atan2(imaginarySum, realSum)

	if angle < 0 {
		angle += 2 * math.Pi
	}

	return angle / (2 * math.Pi)
}

func scalarUnit(value float64) float64 {
	if value >= 0 && value <= 1 {
		return value
	}

	return 0.5 + math.Atan(value)/math.Pi
}

func quantizeUnit(value float64) uint64 {
	value = min(max(value, 0), 1)

	return uint64(math.Round(value * float64(mortonCoordinateMax)))
}
