package geometry

// import (
// 	"encoding/binary"
//
// 	"github.com/theapemachine/six/pkg/logic/lang/primitive"
// 	"github.com/theapemachine/six/pkg/store/data"
// 	"github.com/theapemachine/six/pkg/store/dmt/server"
// )
//
// var morton = data.NewMortonCoder()
//
// /*
// PhaseDialScanner provides PhaseDial-ranked retrieval over a SpatialIndexServer.
// It builds PhaseDial fingerprints from stored value sequences and supports
// geodesic scan, two-hop composition, and ranked similarity queries.
//
// This is the bridge between the pure PhaseDial math (geometry/phase.go) and
// the substrate storage (lsm/spatial_index.go). The old HybridSubstrate combined
// both roles; this design keeps them separate.
// */
// type PhaseDialScanner struct {
// 	substrate *server.ForestServer
// 	cache     map[uint64]cachedEntry
// }
//
// /*
// cachedEntry stores a pre-computed PhaseDial alongside the value sequence
// that produced it. Computing PhaseDials is expensive (O(N * NBasis) per entry),
// so we cache aggressively.
// */
// type cachedEntry struct {
// 	Values []primitive.Value
// 	Dial   PhaseDial
// }
//
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
//
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
//
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
//
// /*
// NewPhaseDialScanner creates a scanner attached to a *server.ForestServer.
// It eagerly builds and caches PhaseDials by calling buildCache during construction.
// */
// func NewPhaseDialScanner(substrate *server.ForestServer) *PhaseDialScanner {
// 	scanner := &PhaseDialScanner{
// 		substrate: substrate,
// 		cache:     make(map[uint64]cachedEntry),
// 	}
//
// 	scanner.buildCache()
//
// 	return scanner
// }
//
// /*
// buildCache materialises PhaseDials for all entries in the substrate.
// Each position in the positionIndex maps to a set of Morton keys; the value
// sequence at each key is the collision chain rooted there.
// */
// func (scanner *PhaseDialScanner) buildCache() {
// 	if scanner.substrate == nil {
// 		return
// 	}
//
// 	forest := scanner.substrate.Forest()
// 	if forest == nil {
// 		return
// 	}
//
// 	forest.Iterate(func(keyBytes []byte, _ []byte) bool {
// 		if len(keyBytes) != 8 {
// 			return true
// 		}
//
// 		mortonKey := binary.BigEndian.Uint64(keyBytes)
//
// 		if _, cached := scanner.cache[mortonKey]; cached {
// 			return true
// 		}
//
// 		_, sym := morton.Unpack(mortonKey)
// 		value := primitive.BaseValue(sym)
// 		values := []primitive.Value{primitive.Value(value)}
// 		dial := NewPhaseDial()
// 		dial = dial.EncodeFromValues(values)
//
// 		scanner.cache[mortonKey] = cachedEntry{
// 			Values: values,
// 			Dial:   dial,
// 		}
//
// 		return true
// 	})
// }
//
// /*
// InvalidateCache clears the PhaseDial cache. Call after substrate mutations
// (insertions, compaction) to force recomputation on next scan.
// */
// func (scanner *PhaseDialScanner) InvalidateCache() {
// 	scanner.cache = make(map[uint64]cachedEntry)
// }
//
// /*
// EntryCount returns the number of cached PhaseDial entries.
// */
// func (scanner *PhaseDialScanner) EntryCount() int {
// 	scanner.buildCache()
// 	return len(scanner.cache)
// }
//
// /*
// EntryDial returns the PhaseDial for a specific Morton key.
// Returns nil if the key does not exist.
// */
// func (scanner *PhaseDialScanner) EntryDial(key uint64) PhaseDial {
// 	scanner.buildCache()
//
// 	entry, exists := scanner.cache[key]
// 	if !exists {
// 		return nil
// 	}
//
// 	return entry.Dial
// }
//
// /*
// EntryValues returns the value sequence for a specific Morton key.
// */
// func (scanner *PhaseDialScanner) EntryValues(key uint64) []primitive.Value {
// 	scanner.buildCache()
//
// 	entry, exists := scanner.cache[key]
// 	if !exists {
// 		return nil
// 	}
//
// 	return entry.Values
// }
