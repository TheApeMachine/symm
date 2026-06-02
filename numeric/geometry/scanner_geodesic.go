package geometry

// import (
// 	"math"
// 	"sort"
// )
//
// /*
// GeodesicScan rotates the seed PhaseDial through [0°, 360°) in nSteps
// equal increments. At each angle the scanner finds the best-matching
// substrate entry. The result is the resonance landscape of the manifold:
// which entries are "visible" from each angular perspective.
// */
// func (scanner *PhaseDialScanner) GeodesicScan(
// 	seedDial PhaseDial, nSteps int,
// ) []GeodesicStep {
// 	scanner.buildCache()
//
// 	if nSteps <= 0 {
// 		nSteps = 72
// 	}
//
// 	stepRad := (2 * math.Pi) / float64(nSteps)
// 	steps := make([]GeodesicStep, nSteps)
//
// 	for i := range nSteps {
// 		angle := float64(i) * stepRad
// 		rotated := seedDial.Rotate(angle)
// 		top := scanner.Scan(rotated, 1)
//
// 		step := GeodesicStep{
// 			AngleDeg: float64(i) * (360.0 / float64(nSteps)),
// 		}
//
// 		if len(top) > 0 {
// 			step.BestKey = top[0].Key
// 			step.Similarity = top[0].Similarity
// 			step.Values = top[0].Values
// 			step.Dial = top[0].Dial
// 		}
//
// 		steps[i] = step
// 	}
//
// 	return steps
// }
//
// /*
// GeodesicScanFull returns the full similarity matrix: for each angular step,
// the similarity to every substrate entry. Rows are entries, columns are angles.
// This produces the heatmap from Figure 3 of the paper.
// */
// func (scanner *PhaseDialScanner) GeodesicScanFull(
// 	seedDial PhaseDial, nSteps int,
// ) (entryKeys []uint64, matrix [][]float64) {
// 	scanner.buildCache()
//
// 	if nSteps <= 0 {
// 		nSteps = 72
// 	}
//
// 	entryKeys = make([]uint64, 0, len(scanner.cache))
// 	for key := range scanner.cache {
// 		entryKeys = append(entryKeys, key)
// 	}
//
// 	sort.Slice(entryKeys, func(i, j int) bool {
// 		return entryKeys[i] < entryKeys[j]
// 	})
//
// 	matrix = make([][]float64, len(entryKeys))
// 	for row := range matrix {
// 		matrix[row] = make([]float64, nSteps)
// 	}
//
// 	stepRad := (2 * math.Pi) / float64(nSteps)
//
// 	for col := range nSteps {
// 		angle := float64(col) * stepRad
// 		rotated := seedDial.Rotate(angle)
//
// 		for row, key := range entryKeys {
// 			matrix[row][col] = rotated.Similarity(scanner.cache[key].Dial)
// 		}
// 	}
//
// 	return entryKeys, matrix
// }
