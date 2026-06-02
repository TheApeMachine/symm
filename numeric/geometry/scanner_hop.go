package geometry

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
//
// 	if len(candidates) == 0 {
// 		return nil
// 	}
//
// 	best := candidates[0]
// 	midpoint := seedDial.ComposeMidpoint(best.Dial)
//
// 	return &HopResult{
// 		KeyB:       best.Key,
// 		DialB:      best.Dial,
// 		DialAB:     midpoint,
// 		ValuesB:    best.Values,
// 		Similarity: best.Similarity,
// 	}
// }
//
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
//
// 	candidates := scanner.ScanExcluding(hop.DialAB, topK, seedKey, hop.KeyB)
//
// 	return hop, candidates
// }
