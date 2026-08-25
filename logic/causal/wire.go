package causal

import (
	"sort"
	"time"

	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

// CausalWire assembles the dashboard causal frame row from a causal output map.
// It is exported so the workspace observer (boot) can own the UI side-effect.
func CausalWire(row map[string]any) *wire.CausalT {
	return &wire.CausalT{
		Source: stringField(row, "source"), Symbol: stringField(row, "symbol"),
		At: timeField(row, "at"), Samples: int64(numberField(row, "samples")),
		Precision: numberField(row, "precision"), Hypothesis: stringField(row, "hypothesis"),
		Treatment: stringField(row, "treatment"), Controls: stringsField(row, "controls"),
		Target: stringField(row, "target"), Value: numberField(row, "value"),
		Category: numberField(row, "category"), Confidence: numberField(row, "confidence"),
		ConfidenceBaseline: numberField(row, "confidenceBaseline"), EntryBaseline: numberField(row, "entryBaseline"),
		ExitBaseline: numberField(row, "exitBaseline"), Strength: numberField(row, "strength"),
		Association: numberField(row, "association"), AssociationScore: numberField(row, "associationScore"),
		Intervention: numberField(row, "intervention"), InterventionScore: numberField(row, "interventionScore"),
		DoExpectation: numberField(row, "doExpectation"), Uplift: numberField(row, "uplift"),
		UpliftScore: numberField(row, "upliftScore"), Residual: numberField(row, "residual"),
		Counterfactual: numberField(row, "counterfactual"), Noise: numberField(row, "noise"),
		NoiseScore: numberField(row, "noiseScore"), Contagion: numberField(row, "contagion"),
		Condition: numberField(row, "condition"), Inverted: boolField(row, "inverted"),
		Probabilities: numbersField(row, "probabilities"), Distribution: namedField(row, "distribution"),
	}
}

func numberField(row map[string]any, name string) float64 {
	switch value := row[name].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case uint64:
		return float64(value)
	default:
		return 0
	}
}

func stringField(row map[string]any, name string) string {
	value, _ := row[name].(string)
	return value
}

func boolField(row map[string]any, name string) bool {
	value, _ := row[name].(bool)
	return value
}

func timeField(row map[string]any, name string) int64 {
	value, _ := row[name].(time.Time)

	if value.IsZero() {
		return 0
	}

	return value.UnixNano()
}

func stringsField(row map[string]any, name string) []string {
	value, _ := row[name].([]string)
	return value
}

func numbersField(row map[string]any, name string) []float64 {
	value, _ := row[name].([]float64)
	return value
}

func namedField(row map[string]any, name string) []*wire.NamedNumberT {
	values, _ := row[name].(map[string]float64)
	names := make([]string, 0, len(values))

	for key := range values {
		names = append(names, key)
	}

	sort.Strings(names)
	result := make([]*wire.NamedNumberT, 0, len(names))

	for _, key := range names {
		result = append(result, &wire.NamedNumberT{Name: key, Value: values[key]})
	}

	return result
}
