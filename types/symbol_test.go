package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSymbolNewSymbol(t *testing.T) {
	Convey("Given a symbol name", t, func() {
		symbol := NewSymbol("BTC/USD", nil)

		Convey("It should initialize empty symbol measurement state", func() {
			So(symbol.Symbol, ShouldEqual, "BTC/USD")
			So(symbol.Status, ShouldEqual, READY)
			So(symbol.Measurements, ShouldBeEmpty)
		})
	})
}

func TestSymbolAddMeasurement(t *testing.T) {
	Convey("Given measurements with one matching source identity", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		first := &Measurement{ID: "first", Source: SourceHawkes, Symbol: "BTC/USD"}
		updated := &Measurement{ID: "updated", Source: SourceHawkes, Symbol: "BTC/USD"}
		peer := &Measurement{
			ID: "peer", Source: SourceHawkes, Symbol: "BTC/USD", Peer: "ETH/USD",
		}

		symbol.AddMeasurement(first)
		symbol.AddMeasurement(peer)
		symbol.AddMeasurement(updated)

		Convey("It should replace only the matching source and peer", func() {
			So(symbol.Status, ShouldEqual, READY)
			So(symbol.Measurements, ShouldResemble, []*Measurement{updated, peer})
		})
	})
}

func TestSymbolStamp(t *testing.T) {
	Convey("Given logic stamps on an active symbol contribution", t, func() {
		symbol := NewSymbol("BTC/USD", nil)
		sources := []SourceType{
			SourceCorrelation, SourceCVD, SourceDepthFlow, SourceExhaustion, SourceHawkes,
			SourceLeadLag, SourceLiquidity, SourcePumpDump, SourceSentiment, SourceToxicity,
		}

		for _, source := range sources {
			symbol.AddMeasurement(&Measurement{
				ID: string(source), Source: source, Symbol: "BTC/USD",
			})
		}

		symbol.Stamp(SourceCategory)

		Convey("It should retain the cut until resonance also finishes", func() {
			So(symbol.Status, ShouldEqual, BUSY)
			So(symbol.Measurements, ShouldHaveLength, len(sources))
		})

		symbol.Stamp(SourceResonance)
		symbol.Stamp(SourceManifold)
		symbol.Stamp(SourceCognition)
		symbol.Stamp(SourceCausal)

		Convey("It should retain the lock until graph completes the analyzer cut", func() {
			So(symbol.Status, ShouldEqual, BUSY)
			So(symbol.Measurements, ShouldHaveLength, len(sources))
		})

		symbol.Stamp(SourceGraph)

		Convey("It should release measurements only after every logic stage stamps", func() {
			So(symbol.Status, ShouldEqual, READY)
			So(symbol.Measurements, ShouldBeEmpty)
			So(symbol.Stamped(SourceCategory), ShouldBeTrue)
			So(symbol.Stamped(SourceResonance), ShouldBeTrue)
			So(symbol.SignalsMeasured(), ShouldBeFalse)
		})
	})
}

func BenchmarkSymbolAddMeasurement(b *testing.B) {
	measurement := &Measurement{
		ID: "hawkes", Source: SourceHawkes, Symbol: "BTC/USD",
	}
	b.ReportAllocs()

	for b.Loop() {
		symbol := NewSymbol("BTC/USD", nil)
		symbol.AddMeasurement(measurement)
	}
}
