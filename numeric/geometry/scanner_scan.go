package geometry

// import (
// 	"sort"
//
// 	"github.com/theapemachine/six/pkg/store/data"
// )
//
// var morton = data.NewMortonCoder()
//
// /*
// Scan ranks all substrate entries by cosine similarity to queryDial.
// Returns the top-K results sorted by descending similarity.
// */
// func (scanner *PhaseDialScanner) Scan(queryDial PhaseDial, topK int) []ScanResult {
// 	scanner.buildCache()
//
// 	results := make([]ScanResult, 0, len(scanner.cache))
//
// 	for key, entry := range scanner.cache {
// 		sim := queryDial.Similarity(entry.Dial)
// 		pos, sym := morton.Unpack(key)
//
// 		results = append(results, ScanResult{
// 			Key:        key,
// 			Position:   pos,
// 			Symbol:     sym,
// 			Similarity: sim,
// 			Values:     entry.Values,
// 			Dial:       entry.Dial,
// 		})
// 	}
//
// 	sort.Slice(results, func(i, j int) bool {
// 		return results[i].Similarity > results[j].Similarity
// 	})
//
// 	if topK > 0 && topK < len(results) {
// 		results = results[:topK]
// 	}
//
// 	return results
// }
//
// /*
// ScanExcluding ranks all entries excluding the specified keys.
// Used for two-hop composition where the seed and first-hop entries
// must be excluded from the second-hop search.
// */
// func (scanner *PhaseDialScanner) ScanExcluding(
// 	queryDial PhaseDial, topK int, excludeKeys ...uint64,
// ) []ScanResult {
// 	scanner.buildCache()
//
// 	excluded := make(map[uint64]bool, len(excludeKeys))
// 	for _, key := range excludeKeys {
// 		excluded[key] = true
// 	}
//
// 	results := make([]ScanResult, 0, len(scanner.cache))
//
// 	for key, entry := range scanner.cache {
// 		if excluded[key] {
// 			continue
// 		}
//
// 		sim := queryDial.Similarity(entry.Dial)
// 		pos, sym := morton.Unpack(key)
//
// 		results = append(results, ScanResult{
// 			Key:        key,
// 			Position:   pos,
// 			Symbol:     sym,
// 			Similarity: sim,
// 			Values:     entry.Values,
// 			Dial:       entry.Dial,
// 		})
// 	}
//
// 	sort.Slice(results, func(i, j int) bool {
// 		return results[i].Similarity > results[j].Similarity
// 	})
//
// 	if topK > 0 && topK < len(results) {
// 		results = results[:topK]
// 	}
//
// 	return results
// }
