package geometry

// import (
// 	"math"
// )
//
// /*
// Steerability measures how much the top-K retrieval set changes when
// a sub-block of the PhaseDial is independently rotated. High steerability
// means the sub-block controls an independent degree of freedom.
// Uses Jaccard distance between consecutive angle steps.
// */
// func (scanner *PhaseDialScanner) Steerability(
// 	dial PhaseDial, blockStart, blockEnd, nAngles, topK int,
// ) float64 {
// 	if nAngles <= 1 || topK <= 0 {
// 		return 0
// 	}
//
// 	topKSets := make([]map[uint64]bool, nAngles)
//
// 	for i := range nAngles {
// 		alpha := float64(i) * (2.0 * math.Pi / float64(nAngles))
// 		rotated := rotateBlock(dial, alpha, blockStart, blockEnd)
// 		results := scanner.Scan(rotated, topK)
//
// 		topKSets[i] = make(map[uint64]bool, len(results))
// 		for _, result := range results {
// 			topKSets[i][result.Key] = true
// 		}
// 	}
//
// 	sumJaccard := 0.0
//
// 	for i := range nAngles {
// 		next := (i + 1) % nAngles
// 		sumJaccard += jaccardDistance(topKSets[i], topKSets[next])
// 	}
//
// 	return sumJaccard / float64(nAngles)
// }
//
// /*
// rotateBlock applies a phase rotation only to dimensions [start, end).
// The rest of the dial is unchanged.
// */
// func rotateBlock(dial PhaseDial, alpha float64, start, end int) PhaseDial {
// 	out := make(PhaseDial, len(dial))
// 	copy(out, dial)
//
// 	f := complex(math.Cos(alpha), math.Sin(alpha))
//
// 	for k := start; k < end && k < len(out); k++ {
// 		out[k] = dial[k] * f
// 	}
//
// 	return out
// }
//
// /*
// jaccardDistance returns 1 - |A∩B|/|A∪B|.
// */
// func jaccardDistance(setA, setB map[uint64]bool) float64 {
// 	inter := 0
//
// 	for key := range setA {
// 		if setB[key] {
// 			inter++
// 		}
// 	}
//
// 	union := len(setA) + len(setB) - inter
// 	if union == 0 {
// 		return 0
// 	}
//
// 	return 1.0 - float64(inter)/float64(union)
// }
