package geometry

// import (
// 	"math"
// 	"testing"

// 	gc "github.com/smartystreets/goconvey/convey"
// 	"github.com/theapemachine/six/pkg/logic/lang/primitive"
// 	"github.com/theapemachine/six/pkg/store/dmt/server"
// )

// func TestPhaseDialScanner(t *testing.T) {
// 	gc.Convey("Given a PhaseDialScanner attached to a populated ForestServer", t, func() {
// 		scanner := NewPhaseDialScanner(server.NewForestServer())

// 		// Insert distinct value sequences at different positions.
// 		// Each position gets a unique value so the PhaseDials differ.
// 		for pos := range uint32(5) {
// 			sym := byte(65 + pos)
// 			values := []primitive.Value{primitive.BaseValue(byte(sym))}
// 			scanner.cache[morton.Pack(pos, sym)] = cachedEntry{
// 				Values: values,
// 				Dial:   NewPhaseDial().EncodeFromValues(values),
// 			}
// 		}

// 		gc.Convey("It should cache all entries", func() {
// 			gc.So(scanner.EntryCount(), gc.ShouldEqual, 5)
// 		})

// 		gc.Convey("EntryDial should return a non-nil PhaseDial for existing keys", func() {
// 			key := morton.Pack(0, 65)
// 			dial := scanner.EntryDial(key)
// 			gc.So(dial, gc.ShouldNotBeNil)
// 			gc.So(len(dial), gc.ShouldBeGreaterThan, 0)
// 		})

// 		gc.Convey("EntryDial should return nil for non-existent keys", func() {
// 			dial := scanner.EntryDial(999999)
// 			gc.So(dial, gc.ShouldBeNil)
// 		})

// 		gc.Convey("Scan should return entries ranked by similarity", func() {
// 			seedKey := morton.Pack(0, 65)
// 			seedDial := scanner.EntryDial(seedKey)
// 			results := scanner.Scan(seedDial, 3)

// 			gc.So(len(results), gc.ShouldBeGreaterThan, 0)
// 			gc.So(
// 				results[0].Similarity,
// 				gc.ShouldBeGreaterThanOrEqualTo,
// 				results[len(results)-1].Similarity,
// 			)
// 		})

// 		gc.Convey("Scan with the entry's own dial should rank itself first", func() {
// 			seedKey := morton.Pack(0, 65)
// 			seedDial := scanner.EntryDial(seedKey)
// 			results := scanner.Scan(seedDial, 1)

// 			gc.So(len(results), gc.ShouldEqual, 1)
// 			gc.So(results[0].Key, gc.ShouldEqual, seedKey)
// 			gc.So(results[0].Similarity, gc.ShouldAlmostEqual, 1.0, 0.01)
// 		})

// 		gc.Convey("ScanExcluding should omit the excluded key", func() {
// 			seedKey := morton.Pack(0, 65)
// 			seedDial := scanner.EntryDial(seedKey)
// 			results := scanner.ScanExcluding(seedDial, 5, seedKey)

// 			for _, result := range results {
// 				gc.So(result.Key, gc.ShouldNotEqual, seedKey)
// 			}
// 		})

// 		gc.Convey("GeodesicScan should return nSteps steps", func() {
// 			seedKey := morton.Pack(0, 65)
// 			seedDial := scanner.EntryDial(seedKey)

// 			steps := scanner.GeodesicScan(seedDial, 24)

// 			gc.So(len(steps), gc.ShouldEqual, 24)
// 			gc.So(steps[0].AngleDeg, gc.ShouldAlmostEqual, 0.0, 0.01)
// 			gc.So(steps[0].Similarity, gc.ShouldBeGreaterThan, 0)
// 		})

// 		gc.Convey("GeodesicScanFull should produce an entries×steps matrix", func() {
// 			seedKey := morton.Pack(0, 65)
// 			seedDial := scanner.EntryDial(seedKey)

// 			keys, matrix := scanner.GeodesicScanFull(seedDial, 12)

// 			gc.So(len(keys), gc.ShouldEqual, 5)
// 			gc.So(len(matrix), gc.ShouldEqual, 5)
// 			gc.So(len(matrix[0]), gc.ShouldEqual, 12)
// 		})

// 		gc.Convey("FirstHop should find a match and compose midpoint", func() {
// 			seedKey := morton.Pack(0, 65)
// 			seedDial := scanner.EntryDial(seedKey)

// 			hop := scanner.FirstHop(seedDial, math.Pi/4, seedKey)

// 			gc.So(hop, gc.ShouldNotBeNil)
// 			gc.So(hop.KeyB, gc.ShouldNotEqual, seedKey)
// 			gc.So(hop.DialAB, gc.ShouldNotBeNil)
// 			gc.So(len(hop.DialAB), gc.ShouldBeGreaterThan, 0)
// 		})

// 		gc.Convey("TwoHop should return both hop and second-stage candidates", func() {
// 			seedKey := morton.Pack(0, 65)
// 			seedDial := scanner.EntryDial(seedKey)

// 			hop, candidates := scanner.TwoHop(seedDial, math.Pi/4, seedKey, 3)

// 			gc.So(hop, gc.ShouldNotBeNil)
// 			gc.So(len(candidates), gc.ShouldBeGreaterThan, 0)

// 			for _, candidate := range candidates {
// 				gc.So(candidate.Key, gc.ShouldNotEqual, seedKey)
// 				gc.So(candidate.Key, gc.ShouldNotEqual, hop.KeyB)
// 			}
// 		})

// 		gc.Convey("InvalidateCache should clear the cache", func() {
// 			gc.So(scanner.EntryCount(), gc.ShouldEqual, 5)
// 			scanner.InvalidateCache()
// 			gc.So(len(scanner.cache), gc.ShouldEqual, 0)
// 		})
// 	})
// }

// func BenchmarkPhaseDialScan(b *testing.B) {
// 	scanner := NewPhaseDialScanner(server.NewForestServer())

// 	for pos := range uint32(100) {
// 		sym := byte(pos % 256)
// 		key := morton.Pack(pos, sym)
// 		value := primitive.BaseValue(byte(sym))
// 		values := []primitive.Value{primitive.Value(value)}
// 		scanner.cache[key] = cachedEntry{
// 			Values: values,
// 			Dial:   NewPhaseDial().EncodeFromValues(values),
// 		}
// 	}

// 	queryDial := NewPhaseDial()

// 	for b.Loop() {
// 		scanner.Scan(queryDial, 10)
// 	}
// }

// func BenchmarkGeodesicScan(b *testing.B) {
// 	scanner := NewPhaseDialScanner(server.NewForestServer())

// 	for pos := uint32(0); pos < 50; pos++ {
// 		sym := byte(pos % 256)
// 		key := morton.Pack(pos, sym)
// 		value := primitive.BaseValue(byte(sym))
// 		values := []primitive.Value{primitive.Value(value)}
// 		scanner.cache[key] = cachedEntry{
// 			Values: values,
// 			Dial:   NewPhaseDial().EncodeFromValues(values),
// 		}
// 	}

// 	queryDial := NewPhaseDial()

// 	for b.Loop() {
// 		scanner.GeodesicScan(queryDial, 24)
// 	}
// }
