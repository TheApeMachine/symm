package geometry

// import (
// 	"encoding/binary"
// 	"math"
// 	"sort"

// 	"github.com/theapemachine/six/pkg/logic/lang/primitive"
// 	"github.com/theapemachine/six/pkg/store/data"
// 	"github.com/theapemachine/six/pkg/store/dmt/server"
// )

// var morton = data.NewMortonCoder()

// /*
// PhaseDialScanner provides PhaseDial-ranked retrieval over a SpatialIndexServer.
// It builds PhaseDial fingerprints from stored value sequences and supports
// geodesic scan, two-hop composition, and ranked similarity queries.

// This is the bridge between the pure PhaseDial math (geometry/phase.go) and
// the substrate storage (lsm/spatial_index.go). The old HybridSubstrate combined
// both roles; this design keeps them separate.
// */
// type PhaseDialScanner struct {
// 	substrate *server.ForestServer
// 	cache     map[uint64]cachedEntry
// }

// /*
// cachedEntry stores a pre-computed PhaseDial alongside the value sequence
// that produced it. Computing PhaseDials is expensive (O(N * NBasis) per entry),
// so we cache aggressively.
// */
// type cachedEntry struct {
// 	Values []primitive.Value
// 	Dial   PhaseDial
// }

// /*
// ScanResult is a single entry from a PhaseDial similarity scan.
// */
// type ScanResult struct {
// 	Key        uint64
// 	Position   uint32
// 	Symbol     byte
// 	Similarity float64
// 	Values     []primitive.Value
// 	Dial       PhaseDial
// }

// /*
// GeodesicStep records the best substrate match at one angular increment
// during a geodesic scan of the phase torus.
// */
// type GeodesicStep struct {
// 	AngleDeg   float64
// 	BestKey    uint64
// 	Similarity float64
// 	Values     []primitive.Value
// 	Dial       PhaseDial
// }

// /*
// HopResult captures the output of a single composition hop:
// the matched entry B, the composed midpoint AB, and the similarity score.
// */
// type HopResult struct {
// 	KeyB       uint64
// 	DialB      PhaseDial
// 	DialAB     PhaseDial
// 	ValuesB    []primitive.Value
// 	Similarity float64
// }

// /*
// NewPhaseDialScanner creates a scanner attached to a *server.ForestServer.
// It eagerly builds and caches PhaseDials by calling buildCache during construction.
// */
// func NewPhaseDialScanner(substrate *server.ForestServer) *PhaseDialScanner {
// 	scanner := &PhaseDialScanner{
// 		substrate: substrate,
// 		cache:     make(map[uint64]cachedEntry),
// 	}

// 	scanner.buildCache()

// 	return scanner
// }

// /*
// buildCache materialises PhaseDials for all entries in the substrate.
// Each position in the positionIndex maps to a set of Morton keys; the value
// sequence at each key is the collision chain rooted there.
// */
// func (scanner *PhaseDialScanner) buildCache() {
// 	if scanner.substrate == nil {
// 		return
// 	}

// 	forest := scanner.substrate.Forest()
// 	if forest == nil {
// 		return
// 	}

// 	forest.Iterate(func(keyBytes []byte, _ []byte) bool {
// 		if len(keyBytes) != 8 {
// 			return true
// 		}

// 		mortonKey := binary.BigEndian.Uint64(keyBytes)

// 		if _, cached := scanner.cache[mortonKey]; cached {
// 			return true
// 		}

// 		_, sym := morton.Unpack(mortonKey)
// 		value := primitive.BaseValue(sym)
// 		values := []primitive.Value{primitive.Value(value)}
// 		dial := NewPhaseDial()
// 		dial = dial.EncodeFromValues(values)

// 		scanner.cache[mortonKey] = cachedEntry{
// 			Values: values,
// 			Dial:   dial,
// 		}

// 		return true
// 	})
// }

// /*
// InvalidateCache clears the PhaseDial cache. Call after substrate mutations
// (insertions, compaction) to force recomputation on next scan.
// */
// func (scanner *PhaseDialScanner) InvalidateCache() {
// 	scanner.cache = make(map[uint64]cachedEntry)
// }

// /*
// EntryCount returns the number of cached PhaseDial entries.
// */
// func (scanner *PhaseDialScanner) EntryCount() int {
// 	scanner.buildCache()
// 	return len(scanner.cache)
// }

// /*
// EntryDial returns the PhaseDial for a specific Morton key.
// Returns nil if the key does not exist.
// */
// func (scanner *PhaseDialScanner) EntryDial(key uint64) PhaseDial {
// 	scanner.buildCache()

// 	entry, exists := scanner.cache[key]
// 	if !exists {
// 		return nil
// 	}

// 	return entry.Dial
// }

// /*
// EntryValues returns the value sequence for a specific Morton key.
// */
// func (scanner *PhaseDialScanner) EntryValues(key uint64) []primitive.Value {
// 	scanner.buildCache()

// 	entry, exists := scanner.cache[key]
// 	if !exists {
// 		return nil
// 	}

// 	return entry.Values
// }

// /*
// Scan ranks all substrate entries by cosine similarity to queryDial.
// Returns the top-K results sorted by descending similarity.
// */
// func (scanner *PhaseDialScanner) Scan(queryDial PhaseDial, topK int) []ScanResult {
// 	scanner.buildCache()

// 	results := make([]ScanResult, 0, len(scanner.cache))

// 	for key, entry := range scanner.cache {
// 		sim := queryDial.Similarity(entry.Dial)
// 		pos, sym := morton.Unpack(key)

// 		results = append(results, ScanResult{
// 			Key:        key,
// 			Position:   pos,
// 			Symbol:     sym,
// 			Similarity: sim,
// 			Values:     entry.Values,
// 			Dial:       entry.Dial,
// 		})
// 	}

// 	sort.Slice(results, func(i, j int) bool {
// 		return results[i].Similarity > results[j].Similarity
// 	})

// 	if topK > 0 && topK < len(results) {
// 		results = results[:topK]
// 	}

// 	return results
// }

// /*
// ScanExcluding ranks all entries excluding the specified keys.
// Used for two-hop composition where the seed and first-hop entries
// must be excluded from the second-hop search.
// */
// func (scanner *PhaseDialScanner) ScanExcluding(
// 	queryDial PhaseDial, topK int, excludeKeys ...uint64,
// ) []ScanResult {
// 	scanner.buildCache()

// 	excluded := make(map[uint64]bool, len(excludeKeys))
// 	for _, key := range excludeKeys {
// 		excluded[key] = true
// 	}

// 	results := make([]ScanResult, 0, len(scanner.cache))

// 	for key, entry := range scanner.cache {
// 		if excluded[key] {
// 			continue
// 		}

// 		sim := queryDial.Similarity(entry.Dial)
// 		pos, sym := morton.Unpack(key)

// 		results = append(results, ScanResult{
// 			Key:        key,
// 			Position:   pos,
// 			Symbol:     sym,
// 			Similarity: sim,
// 			Values:     entry.Values,
// 			Dial:       entry.Dial,
// 		})
// 	}

// 	sort.Slice(results, func(i, j int) bool {
// 		return results[i].Similarity > results[j].Similarity
// 	})

// 	if topK > 0 && topK < len(results) {
// 		results = results[:topK]
// 	}

// 	return results
// }

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

// 	if nSteps <= 0 {
// 		nSteps = 72
// 	}

// 	stepRad := (2 * math.Pi) / float64(nSteps)
// 	steps := make([]GeodesicStep, nSteps)

// 	for i := range nSteps {
// 		angle := float64(i) * stepRad
// 		rotated := seedDial.Rotate(angle)
// 		top := scanner.Scan(rotated, 1)

// 		step := GeodesicStep{
// 			AngleDeg: float64(i) * (360.0 / float64(nSteps)),
// 		}

// 		if len(top) > 0 {
// 			step.BestKey = top[0].Key
// 			step.Similarity = top[0].Similarity
// 			step.Values = top[0].Values
// 			step.Dial = top[0].Dial
// 		}

// 		steps[i] = step
// 	}

// 	return steps
// }

// /*
// GeodesicScanFull returns the full similarity matrix: for each angular step,
// the similarity to every substrate entry. Rows are entries, columns are angles.
// This produces the heatmap from Figure 3 of the paper.
// */
// func (scanner *PhaseDialScanner) GeodesicScanFull(
// 	seedDial PhaseDial, nSteps int,
// ) (entryKeys []uint64, matrix [][]float64) {
// 	scanner.buildCache()

// 	if nSteps <= 0 {
// 		nSteps = 72
// 	}

// 	entryKeys = make([]uint64, 0, len(scanner.cache))
// 	for key := range scanner.cache {
// 		entryKeys = append(entryKeys, key)
// 	}

// 	sort.Slice(entryKeys, func(i, j int) bool {
// 		return entryKeys[i] < entryKeys[j]
// 	})

// 	matrix = make([][]float64, len(entryKeys))
// 	for row := range matrix {
// 		matrix[row] = make([]float64, nSteps)
// 	}

// 	stepRad := (2 * math.Pi) / float64(nSteps)

// 	for col := range nSteps {
// 		angle := float64(col) * stepRad
// 		rotated := seedDial.Rotate(angle)

// 		for row, key := range entryKeys {
// 			matrix[row][col] = rotated.Similarity(scanner.cache[key].Dial)
// 		}
// 	}

// 	return entryKeys, matrix
// }

// /*
// FirstHop performs the first hop of two-hop composition.
// Starting from seedDial, it rotates by the given angle, finds the
// best-matching entry B (excluding the seed key), and composes the
// midpoint AB = ComposeMidpoint(seedDial, B.Dial).
// */
// func (scanner *PhaseDialScanner) FirstHop(
// 	seedDial PhaseDial, angleRad float64, seedKey uint64,
// ) *HopResult {
// 	rotated := seedDial.Rotate(angleRad)
// 	candidates := scanner.ScanExcluding(rotated, 1, seedKey)

// 	if len(candidates) == 0 {
// 		return nil
// 	}

// 	best := candidates[0]
// 	midpoint := seedDial.ComposeMidpoint(best.Dial)

// 	return &HopResult{
// 		KeyB:       best.Key,
// 		DialB:      best.Dial,
// 		DialAB:     midpoint,
// 		ValuesB:    best.Values,
// 		Similarity: best.Similarity,
// 	}
// }

// /*
// TwoHop performs two-hop composition: hop A→B, compose midpoint AB,
// then search for C that is simultaneously close to both A and B but
// is neither A nor B. Returns the hop result for each stage and the
// final C candidates.
// */
// func (scanner *PhaseDialScanner) TwoHop(
// 	seedDial PhaseDial, hopAngleRad float64, seedKey uint64, topK int,
// ) (*HopResult, []ScanResult) {
// 	hop := scanner.FirstHop(seedDial, hopAngleRad, seedKey)
// 	if hop == nil {
// 		return nil, nil
// 	}

// 	candidates := scanner.ScanExcluding(hop.DialAB, topK, seedKey, hop.KeyB)

// 	return hop, candidates
// }

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

// 	topKSets := make([]map[uint64]bool, nAngles)

// 	for i := range nAngles {
// 		alpha := float64(i) * (2.0 * math.Pi / float64(nAngles))
// 		rotated := rotateBlock(dial, alpha, blockStart, blockEnd)
// 		results := scanner.Scan(rotated, topK)

// 		topKSets[i] = make(map[uint64]bool, len(results))
// 		for _, result := range results {
// 			topKSets[i][result.Key] = true
// 		}
// 	}

// 	sumJaccard := 0.0

// 	for i := range nAngles {
// 		next := (i + 1) % nAngles
// 		sumJaccard += jaccardDistance(topKSets[i], topKSets[next])
// 	}

// 	return sumJaccard / float64(nAngles)
// }

// /*
// rotateBlock applies a phase rotation only to dimensions [start, end).
// The rest of the dial is unchanged.
// */
// func rotateBlock(dial PhaseDial, alpha float64, start, end int) PhaseDial {
// 	out := make(PhaseDial, len(dial))
// 	copy(out, dial)

// 	f := complex(math.Cos(alpha), math.Sin(alpha))

// 	for k := start; k < end && k < len(out); k++ {
// 		out[k] = dial[k] * f
// 	}

// 	return out
// }

// /*
// jaccardDistance returns 1 - |A∩B|/|A∪B|.
// */
// func jaccardDistance(setA, setB map[uint64]bool) float64 {
// 	inter := 0

// 	for key := range setA {
// 		if setB[key] {
// 			inter++
// 		}
// 	}

// 	union := len(setA) + len(setB) - inter
// 	if union == 0 {
// 		return 0
// 	}

// 	return 1.0 - float64(inter)/float64(union)
// }
