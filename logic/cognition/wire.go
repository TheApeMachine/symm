package cognition

import (
	"sort"
	"time"

	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

// CognitionWire assembles the dashboard cognition row for a reading. It is
// exported so the workspace observer (boot) can own the UI side-effect.
func CognitionWire(reading types.Cognition) *wire.CognitionT {
	branches := make([]*wire.CognitionBranchT, 0, len(reading.Branches))

	for _, branch := range reading.Branches {
		branches = append(branches, &wire.CognitionBranchT{
			Id: int64(branch.ID), ParentId: int64(branch.ParentID), Token: branch.Token,
			Prefix: branch.Prefix, Key: branch.Key, Depth: int64(branch.Depth),
			Probability: branch.Probability, Count: branch.Count,
		})
	}

	beams := make([]*wire.CognitionBeamT, 0, len(reading.Beams))

	for _, beam := range reading.Beams {
		beams = append(beams, &wire.CognitionBeamT{
			Sequence: beam.Sequence, Key: beam.Key, Score: beam.Score,
		})
	}

	classes := make([]*wire.CognitionClassT, 0, len(reading.Classes))

	for _, class := range reading.Classes {
		classes = append(classes, &wire.CognitionClassT{
			Name: class.Name, Probability: class.Probability,
		})
	}

	contributions := make([]*wire.CognitionContributionT, 0, len(reading.Contributions))

	for _, contribution := range reading.Contributions {
		contributions = append(contributions, &wire.CognitionContributionT{
			Token: contribution.Token, Bits: contribution.Bits,
		})
	}

	symbols := make([]*wire.CognitionSymbolT, 0, len(reading.Symbols))

	for _, symbol := range reading.Symbols {
		symbols = append(symbols, &wire.CognitionSymbolT{
			Symbol: symbol.Symbol, ClassName: symbol.Class,
			Score: symbol.Score, Purity: symbol.Purity,
		})
	}

	lexical := make([]*wire.CognitionLexicalT, 0, len(reading.Lexical))

	for _, token := range reading.Lexical {
		lexical = append(lexical, &wire.CognitionLexicalT{
			Original: token.Original, Mapped: token.Mapped, Similarity: token.Similarity,
		})
	}

	return &wire.CognitionT{
		Source: reading.Source, Symbol: reading.Symbol, At: timestamp(reading.At),
		Sequence: reading.Sequence, RegimePrefix: reading.RegimePrefix,
		Winner: reading.Winner, WinnerClass: reading.WinnerClass,
		CandidateWinner: reading.CandidateWinner, StateHeld: reading.StateHeld,
		PredictionsHeld: reading.PredictionsHeld, SwitchConfidence: reading.SwitchConfidence,
		SwitchThreshold: reading.SwitchThreshold, Error: reading.Error,
		Confidence: reading.Confidence, ClassConfidence: reading.ClassConfidence,
		Contrast: reading.Contrast, ContrastEvidence: reading.ContrastEvidence,
		EntropyBits: floatValue(reading.EntropyBits), HasEntropyBits: reading.EntropyBits != nil,
		EntropyThreshold: floatValue(reading.EntropyThreshold), HasEntropyThreshold: reading.EntropyThreshold != nil,
		Ambiguous: reading.Ambiguous, Cohort: reading.Cohort,
		LookaheadScore: reading.LookaheadScore, LookaheadPaths: int64(reading.LookaheadPaths),
		BeamWidth: int64(reading.BeamWidth), MaxHops: int64(reading.MaxHops), NodeCount: int64(reading.NodeCount),
		Predictions: cognitionNumbers(reading.Predictions), Branches: branches,
		Beams: beams, Classes: classes, RemFrom: timestamp(reading.REMFrom),
		RemThrough: timestamp(reading.REMThrough), RemReplays: int64(reading.REMReplays),
		RemDecayFactor: reading.REMDecayFactor, RemInhibitionPct: reading.REMInhibitionPct,
		RemConsolidating: reading.REMConsolidating, InterpolatedSurprisal: reading.InterpolatedSurprisal,
		Contributions: contributions, Symbols: symbols, Lexical: lexical, Dreams: reading.Dreams,
	}
}

func cognitionNumbers(values map[string]float64) []*wire.NamedNumberT {
	names := make([]string, 0, len(values))

	for name := range values {
		names = append(names, name)
	}

	sort.Strings(names)
	result := make([]*wire.NamedNumberT, 0, len(names))

	for _, name := range names {
		result = append(result, &wire.NamedNumberT{Name: name, Value: values[name]})
	}

	return result
}

func floatValue(value *float64) float64 {
	if value == nil {
		return 0
	}

	return *value
}

func timestamp(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}

	return at.UnixNano()
}
