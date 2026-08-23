package algo

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolChange            = nomagique.MustIntern("sentiment/change")
	SymbolBreadth           = nomagique.MustIntern("sentiment/breadth")
	SymbolSurgeScore        = nomagique.MustIntern("sentiment/surge_score")
	SymbolSlumpScore        = nomagique.MustIntern("sentiment/slump_score")
	SymbolDivergentScore    = nomagique.MustIntern("sentiment/divergent_score")
	SymbolLeaderStrength    = nomagique.MustIntern("sentiment/leader_strength")
	SymbolLeaderEvidence    = nomagique.MustIntern("sentiment/leader_evidence")
	SymbolRelativeLead      = nomagique.MustIntern("sentiment/relative_lead")
	SymbolSentimentStrength = nomagique.MustIntern("sentiment/strength")
	SymbolCohortPeerCount   = nomagique.MustIntern("sentiment/peer_count")
	SymbolSentimentReady    = nomagique.MustIntern("sentiment/ready")
)

type peerReturn struct {
	symbol    string
	change    float64
	magnitude float64
}

/*
CohortSentiment evaluates cross-sectional breadth, directional surge/slump agreement,
leadership emergence, and leader-cohort divergence across all ready paths in the Number.
*/
func CohortSentiment(
	focalKey string,
	number *nomagique.Number[string],
) (types.Frame, bool, error) {
	output := types.Frame{}

	if number == nil {
		return output, false, nil
	}

	focalPath, hasFocal := number.Project(focalKey)

	if !hasFocal {
		return output, false, nil
	}

	_, focalReturnFrame, err := nomagique.Step(
		correlation.Return, types.Frame{}, focalPath,
	)

	if err != nil {
		return output, false, err
	}

	focalReady, _ := focalReturnFrame.Get(correlation.SymbolReady)
	focalChange, _ := focalReturnFrame.Get(correlation.SymbolReturn)

	if focalReady != 0 {
		output.Put(SymbolChange, focalChange)
	}

	peers, scanErr := collectPeerReturns(number)

	if scanErr != nil {
		return output, false, scanErr
	}

	if len(peers) == 0 {
		output.Put(SymbolSentimentReady, 0)

		return output, false, nil
	}

	computeSentimentScores(focalKey, peers, &output)

	return output, true, nil
}

func collectPeerReturns(
	number *nomagique.Number[string],
) ([]peerReturn, error) {
	peers := make([]peerReturn, 0)
	var scanErr error

	number.Range(func(key string, state types.Frame) bool {
		_, returnFrame, returnErr := nomagique.Step(
			correlation.Return, types.Frame{}, state,
		)

		if returnErr != nil {
			scanErr = returnErr

			return false
		}

		ready, hasReady := returnFrame.Get(correlation.SymbolReady)

		if !hasReady || ready == 0 {
			return true
		}

		change := returnFrame.MustGet(correlation.SymbolReturn)
		magnitude := returnFrame.MustGet(correlation.SymbolMagnitude)

		peers = append(peers, peerReturn{
			symbol:    key,
			change:    change,
			magnitude: magnitude,
		})

		return true
	})

	return peers, scanErr
}

func computeSentimentScores(
	focalKey string,
	peers []peerReturn,
	output *types.Frame,
) {
	count := float64(len(peers))
	changes, magnitudes, totalMag, leaderIndex, leaderMag, advances, declines := scanPeers(peers)

	breadth := (advances - declines) / count
	medianMag := medianSlice(magnitudes)
	medianChg := medianSlice(changes)
	agreement := math.Max(advances, declines) / count
	surge, slump := calculateSurgeSlump(medianChg, medianMag, agreement)

	relativeLead := 0.0

	if totalMag > 0 {
		relativeLead = leaderMag / totalMag
	}

	leaderSymbol := peers[leaderIndex].symbol
	peerMagnitudes := extractPeerMagnitudes(magnitudes, leaderIndex)
	leaderEvidence := calculateLeaderEvidence(leaderMag, peerMagnitudes)
	divergence := calculateDivergence(
		peers[leaderIndex].change, leaderEvidence, peers, leaderIndex, len(peerMagnitudes),
	)
	strength := math.Max(surge, math.Max(slump, divergence))

	output.Put(SymbolBreadth, breadth)
	output.Put(SymbolSurgeScore, surge)
	output.Put(SymbolSlumpScore, slump)
	output.Put(SymbolSentimentStrength, strength)
	output.Put(SymbolCohortPeerCount, count)
	output.Put(SymbolSentimentReady, 1)

	if focalKey == leaderSymbol {
		output.Put(SymbolLeaderStrength, leaderMag)
		output.Put(SymbolLeaderEvidence, leaderEvidence)
		output.Put(SymbolRelativeLead, relativeLead)
		output.Put(SymbolDivergentScore, divergence)
	}
}

func scanPeers(
	peers []peerReturn,
) ([]float64, []float64, float64, int, float64, float64, float64) {
	changes := make([]float64, len(peers))
	magnitudes := make([]float64, len(peers))
	totalMag := 0.0
	leaderIndex := 0
	leaderMag := 0.0
	advances := 0.0
	declines := 0.0

	for index, peer := range peers {
		changes[index] = peer.change
		magnitudes[index] = peer.magnitude
		totalMag += peer.magnitude

		if peer.change > 0 {
			advances++
		}

		if peer.change < 0 {
			declines++
		}

		if peer.magnitude > leaderMag {
			leaderMag = peer.magnitude
			leaderIndex = index
		}
	}

	return changes, magnitudes, totalMag, leaderIndex, leaderMag, advances, declines
}

func calculateSurgeSlump(
	medianChg float64,
	medianMag float64,
	agreement float64,
) (float64, float64) {
	if medianMag <= 0 {
		return 0, 0
	}

	surge := math.Max(0, medianChg) * agreement / medianMag
	slump := math.Max(0, -medianChg) * agreement / medianMag

	return surge, slump
}

func extractPeerMagnitudes(
	magnitudes []float64,
	leaderIndex int,
) []float64 {
	peerMagnitudes := append([]float64(nil), magnitudes...)

	return append(peerMagnitudes[:leaderIndex], peerMagnitudes[leaderIndex+1:]...)
}

func calculateLeaderEvidence(
	leaderMag float64,
	peerMagnitudes []float64,
) float64 {
	if len(peerMagnitudes) == 0 {
		return 0
	}

	peerMedian := medianSlice(peerMagnitudes)

	if leaderMag <= peerMedian {
		return 0
	}

	deviations := make([]float64, len(peerMagnitudes))

	for index, mag := range peerMagnitudes {
		deviations[index] = math.Abs(mag - peerMedian)
	}

	peerDispersion := medianSlice(deviations)
	excess := leaderMag - peerMedian

	return excess / (excess + peerDispersion)
}

func calculateDivergence(
	leaderChange float64,
	leaderEvidence float64,
	peers []peerReturn,
	leaderIndex int,
	peerCount int,
) float64 {
	if leaderEvidence <= 0 || peerCount == 0 {
		return 0
	}

	leaderSign := math.Copysign(1, leaderChange)
	nonconfirming := 0.0

	for index, peer := range peers {
		if index == leaderIndex || peer.change == 0 {
			continue
		}

		if math.Copysign(1, peer.change) != leaderSign {
			nonconfirming++
		}
	}

	return leaderEvidence * nonconfirming / float64(peerCount)
}

func medianSlice(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2

	if len(sorted)%2 == 0 {
		return (sorted[middle-1] + sorted[middle]) / 2
	}

	return sorted[middle]
}
