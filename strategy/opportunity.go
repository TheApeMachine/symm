package strategy

import (
	"strings"
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/types"
)

/*
OpportunityInput gathers the deliberated consensus and market context necessary
to evaluate whether an actionable opportunity exists.
*/
type OpportunityInput struct {
	Symbol           string
	Consensus        *advisor.DeliberationOutcome
	Resonance        *types.ResonanceArtifact
	Cognition        *types.Cognition
	Categories       []types.Category
	LiquidationShare float64
	Desk             *broker.Desk
	At               time.Time
}

/*
SynthesizeOpportunity evaluates the complete, quality-adjusted body of evidence to
determine if a coherent, actionable opportunity exists.

It does NOT invent a destination price. Instead, it makes the modest defensible
claim: from the precursors present, what is the probability of upward movement
P(up), and what can reasonably be said about the potential magnitude of that movement?

Bullish evidence is not the same thing as an entry opportunity. A weak upward
tendency, an isolated positive advisor, or a handful of noisy metrics should not
be sufficient reason to spend capital. If the upward potential does not distinctly
clear market noise and frictions, waiting is the ordinary outcome and
SynthesizeOpportunity returns nil.
*/
func SynthesizeOpportunity(input OpportunityInput) *types.OpportunityCandidate {
	consensus := input.Consensus

	if consensus == nil {
		return nil
	}

	// 1. Participant diversity: an isolated advisor or 2-advisor pair is not an entry opportunity.
	// A council requires at least 3 deliberating advisors across 3 independent source groups
	// to prevent entering blindly into thin or exhausted books.
	if consensus.Participants < 3 {
		return nil
	}

	if consensus.IndependentSources > 0 && consensus.IndependentSources < 3 {
		return nil
	}

	// 2. Active vetoes: an unresolved contradiction invalidates the hypothesis.
	if len(consensus.Vetoes) > 0 {
		return nil
	}

	// 3. Probability of upward movement vs downward movement.
	//
	// The direction is read from the AGGREGATED mass, never from
	// DominantMove. DominantMove is the argmax over a move space whose bins
	// are asymmetric: "up" is divided between ExplosivePump and SteadyTrend
	// and "down" across three bins, while Stagnant is the one undivided
	// thesis. A council can therefore place most of its mass on rising prices
	// and still name Stagnant its single heaviest bin, purely because the
	// directional mass is split across two bins and the neutral mass is not.
	// Gating on the argmax vetoed exactly those opportunities.
	probUp := 0.0
	probDown := 0.0
	probPump := 0.0
	probTrend := 0.0

	if len(consensus.Probabilities) > 0 {
		probPump = consensus.Probabilities[advisor.MoveExplosivePump]
		probTrend = consensus.Probabilities[advisor.MoveSteadyTrend]
		probDrift := consensus.Probabilities[advisor.MoveWeakDrift]

		probDump := consensus.Probabilities[advisor.MoveFlashDump]
		probPullback := consensus.Probabilities[advisor.MoveStructuralPullback]
		probBleed := consensus.Probabilities[advisor.MoveWeakBleed]

		probUp = probPump + probTrend + 0.5*probDrift
		probDown = probDump + probPullback + 0.5*probBleed
	}

	// Without a distribution there is no directional evidence at all.
	// Confidence cannot stand in for it: Confidence is the share held by
	// whichever bin is heaviest, and that bin may well be FlashDump, so
	// reading it as "probability up" inverts a bearish council into a bullish
	// one. A real deliberation always publishes Probabilities over every move.
	if len(consensus.Probabilities) == 0 {
		return nil
	}

	// Which directional reading leads the up-case, for the checks below that
	// need to distinguish an explosive move from a steady one. This replaces
	// the DominantMove comparisons, which carried the same argmax bias.
	explosive := probPump > probTrend

	// High liquidation share warns that an explosive pump is an artificial short squeeze
	// that will terminate as soon as forced stops are cleared.
	if input.LiquidationShare > 0.35 && explosive {
		probUp *= (1.0 - input.LiquidationShare)
	}

	// 4. Category regime context: an active collapse, exhaustion, or liquidity vacuum
	// contradicts any entry hypothesis regardless of what short-horizon advisors read.
	if len(input.Categories) > 0 {
		dominant := input.Categories[0]

		if dominant.Type == types.MechanicalCollapse ||
			dominant.Type == types.FadedExhaustion ||
			dominant.Type == types.LiquidityVacuum ||
			dominant.Type == types.ThermalExhaustion ||
			dominant.Type == types.ActiveReversal {
			return nil
		}

		if (dominant.Type == types.VerticalIgnition || dominant.Type == types.OrganicTrend) && dominant.Confidence > 0 {
			probUp = min(0.95, probUp*(1.0+0.10*dominant.Confidence))
		}
	}

	// 5. Cognition lookahead & ambiguity: evaluate DMT regime transitions.
	if input.Cognition != nil {
		if input.Cognition.Ambiguous {
			probUp *= 0.90
		}

		if len(input.Cognition.Predictions) > 0 {
			bullishScore := 0.0
			adverseScore := 0.0

			for path, score := range input.Cognition.Predictions {
				if isBullishRegimePath(path) {
					bullishScore += score
				}

				if isAdverseRegimePath(path) {
					adverseScore += score
				}
			}

			totalScore := bullishScore + adverseScore

			if totalScore > 0 {
				adverseShare := adverseScore / totalScore

				if adverseShare > 0.50 {
					probUp *= (1.0 - adverseShare*0.50)
				}

				bullishShare := bullishScore / totalScore

				if bullishShare > 0.50 {
					probUp = min(0.95, probUp*(1.0+0.10*bullishShare))
				}
			}
		}
	}

	// A weak upward tendency does not justify risk.
	if probUp <= probDown || probUp < 0.50 {
		return nil
	}

	// 6. Magnitude: derived honestly from realized passage economics, never invented.
	magnitude := 0.0

	if input.Desk != nil && input.Symbol != "" {
		passageEconomics := input.Desk.PassageEconomics(input.Symbol)

		if passageEconomics.FavorableExcursion.Mid > 0 {
			magnitude = passageEconomics.FavorableExcursion.Mid
		}

		if passageEconomics.FavorableExcursion.Mid <= 0 {
			magnitude = input.Desk.PassageMovementMagnitude(input.Symbol)
		}
	}

	// Fallback magnitude estimate from dominant move return thresholds when desk is not available (e.g. fixture replay or unit tests)
	if magnitude <= 0 {
		if explosive {
			magnitude = 0.02
		}

		if !explosive {
			magnitude = 0.005
		}
	}

	// 7. Resonance consistency (when present)
	if input.Resonance != nil {
		if !input.Resonance.Calibrated {
			return nil
		}

		if input.Resonance.Forecast == nil || input.Resonance.Forecast.Held || input.Resonance.Forecast.Call <= 0 {
			return nil
		}
	}

	profitFirst := probUp / (probUp + probDown)

	// Uncertainty is measured against the DIRECTIONAL conviction, not against
	// Confidence. Confidence is the argmax bin's share of a seven-bin space,
	// so it is bounded well below 1 even for a unanimous council and would
	// pin uncertainty near a constant regardless of what was actually agreed.
	uncertainty := 1.0 - probUp

	if input.Cognition != nil {
		if input.Cognition.InterpolatedSurprisal > 0 {
			uncertainty += input.Cognition.InterpolatedSurprisal * 0.05
		}

		if input.Cognition.Ambiguous {
			uncertainty += 0.10
		}

		if len(input.Cognition.Predictions) > 0 {
			bullishScore := 0.0
			totalScore := 0.0

			for path, score := range input.Cognition.Predictions {
				if isBullishRegimePath(path) {
					bullishScore += score
				}

				totalScore += score
			}

			if totalScore > 0 {
				bullishShare := bullishScore / totalScore

				if bullishShare > 0.50 {
					uncertainty = max(0.05, uncertainty*(1.0-0.20*bullishShare))
				}
			}
		}
	}

	economics := &types.OpportunityEconomics{
		Calibrated:            true,
		TransitionProbability: probUp,
		ProfitFirst:           profitFirst,
		FavorableExcursion: types.Excursion{
			Mid: magnitude,
		},
		Uncertainty: uncertainty,
	}

	return &types.OpportunityCandidate{
		Symbol:    input.Symbol,
		Archetype: archetypeFor(explosive),
		Phase:     types.PhaseArmed,
		Direction: types.DirectionLong,
		FirstSeen: input.At,
		Updated:   input.At,
		Maturity:  probUp,
		Economics: economics,
	}
}

/*
archetypeFor names the situation the council actually described.

The archetype was previously the constant ArchetypeVerticalIgnition, so every
opportunity this synthesizer produced claimed a vertical break regardless of
what the advisors read — an entry taken on a council whose mass sat on steady
trend still reported itself as an ignition. The label is not decoration: it is
what the decision surface, the trade journal and hindsight all show as the
reason the position exists, so a fixed one makes every entry describe the same
setup and none of them describe the real one.

The distinction is which reading carries the up-case. Explosive pump leading it
is the vertical break the ignition archetype names; steady trend leading it is
a move already underway, which is a weaker and different claim.
*/
func archetypeFor(explosive bool) types.OpportunityArchetype {
	if explosive {
		return types.ArchetypeVerticalIgnition
	}

	return types.ArchetypeSustainedTrend
}

func isBullishRegimePath(path string) bool {
	return strings.Contains(path, string(types.VerticalIgnition)) ||
		strings.Contains(path, string(types.OrganicTrend)) ||
		strings.Contains(path, string(types.LeveragedIgnition)) ||
		strings.Contains(path, string(types.RiskOnSurge)) ||
		strings.Contains(path, string(types.AggressiveDrive))
}

func isAdverseRegimePath(path string) bool {
	return strings.Contains(path, string(types.MechanicalCollapse)) ||
		strings.Contains(path, string(types.FadedExhaustion)) ||
		strings.Contains(path, string(types.LiquidityVacuum)) ||
		strings.Contains(path, string(types.SystemicSlump)) ||
		strings.Contains(path, string(types.ThermalExhaustion)) ||
		strings.Contains(path, string(types.ActiveReversal))
}
