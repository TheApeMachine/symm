package types

import (
	telemetry "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
EncodeWire projects a domain Cognition reading into the FlatBuffer wire representation.
*/
func (cognition *Cognition) EncodeWire() *telemetry.CognitionT {
	if cognition == nil {
		return nil
	}

	var remFromNano int64
	if !cognition.REMFrom.IsZero() {
		remFromNano = cognition.REMFrom.UnixNano()
	}

	var remThroughNano int64
	if !cognition.REMThrough.IsZero() {
		remThroughNano = cognition.REMThrough.UnixNano()
	}

	wire := &telemetry.CognitionT{
		Source:                cognition.Source,
		Symbol:                cognition.Symbol,
		At:                    cognition.At.UnixNano(),
		Sequence:              cognition.Sequence,
		RegimePrefix:          cognition.RegimePrefix,
		Winner:                cognition.Winner,
		WinnerClass:           cognition.WinnerClass,
		CandidateWinner:       cognition.CandidateWinner,
		StateHeld:             cognition.StateHeld,
		PredictionsHeld:       cognition.PredictionsHeld,
		SwitchConfidence:      cognition.SwitchConfidence,
		SwitchThreshold:       cognition.SwitchThreshold,
		Error:                 cognition.Error,
		Confidence:            cognition.Confidence,
		ClassConfidence:       cognition.ClassConfidence,
		Contrast:              cognition.Contrast,
		ContrastEvidence:      cognition.ContrastEvidence,
		Ambiguous:             cognition.Ambiguous,
		Cohort:                cognition.Cohort,
		LookaheadScore:        cognition.LookaheadScore,
		LookaheadPaths:        int64(cognition.LookaheadPaths),
		BeamWidth:             int64(cognition.BeamWidth),
		MaxHops:               int64(cognition.MaxHops),
		NodeCount:             int64(cognition.NodeCount),
		RemFrom:               remFromNano,
		RemThrough:            remThroughNano,
		RemReplays:            int64(cognition.REMReplays),
		RemDecayFactor:        cognition.REMDecayFactor,
		RemInhibitionPct:      cognition.REMInhibitionPct,
		RemConsolidating:      cognition.REMConsolidating,
		InterpolatedSurprisal: cognition.InterpolatedSurprisal,
		Dreams:                cognition.Dreams,
	}

	if cognition.EntropyBits != nil {
		wire.EntropyBits = *cognition.EntropyBits
		wire.HasEntropyBits = true
	}

	if cognition.EntropyThreshold != nil {
		wire.EntropyThreshold = *cognition.EntropyThreshold
		wire.HasEntropyThreshold = true
	}

	if len(cognition.Predictions) > 0 {
		predictions := make([]*telemetry.NamedNumberT, 0, len(cognition.Predictions))
		for name, value := range cognition.Predictions {
			predictions = append(predictions, &telemetry.NamedNumberT{
				Name:  name,
				Value: value,
			})
		}
		wire.Predictions = predictions
	}

	if len(cognition.Branches) > 0 {
		branches := make([]*telemetry.CognitionBranchT, 0, len(cognition.Branches))
		for _, branch := range cognition.Branches {
			branches = append(branches, &telemetry.CognitionBranchT{
				Id:          int64(branch.ID),
				ParentId:    int64(branch.ParentID),
				Token:       branch.Token,
				Prefix:      branch.Prefix,
				Key:         branch.Key,
				Depth:       int64(branch.Depth),
				Probability: branch.Probability,
				Count:       branch.Count,
			})
		}
		wire.Branches = branches
	}

	if len(cognition.Beams) > 0 {
		beams := make([]*telemetry.CognitionBeamT, 0, len(cognition.Beams))
		for _, beam := range cognition.Beams {
			beams = append(beams, &telemetry.CognitionBeamT{
				Sequence: beam.Sequence,
				Key:      beam.Key,
				Score:    beam.Score,
			})
		}
		wire.Beams = beams
	}

	if len(cognition.Classes) > 0 {
		classes := make([]*telemetry.CognitionClassT, 0, len(cognition.Classes))
		for _, class := range cognition.Classes {
			classes = append(classes, &telemetry.CognitionClassT{
				Name:        class.Name,
				Probability: class.Probability,
			})
		}
		wire.Classes = classes
	}

	if len(cognition.Contributions) > 0 {
		contributions := make([]*telemetry.CognitionContributionT, 0, len(cognition.Contributions))
		for _, contribution := range cognition.Contributions {
			contributions = append(contributions, &telemetry.CognitionContributionT{
				Token: contribution.Token,
				Bits:  contribution.Bits,
			})
		}
		wire.Contributions = contributions
	}

	if len(cognition.Symbols) > 0 {
		symbols := make([]*telemetry.CognitionSymbolT, 0, len(cognition.Symbols))
		for _, sym := range cognition.Symbols {
			symbols = append(symbols, &telemetry.CognitionSymbolT{
				Symbol:    sym.Symbol,
				ClassName: sym.Class,
				Score:     sym.Score,
				Purity:    sym.Purity,
			})
		}
		wire.Symbols = symbols
	}

	if len(cognition.Lexical) > 0 {
		lexical := make([]*telemetry.CognitionLexicalT, 0, len(cognition.Lexical))
		for _, lex := range cognition.Lexical {
			lexical = append(lexical, &telemetry.CognitionLexicalT{
				Original:   lex.Original,
				Mapped:     lex.Mapped,
				Similarity: lex.Similarity,
			})
		}
		wire.Lexical = lexical
	}

	return wire
}
